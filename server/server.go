// Package server provides the Fiber HTTP server with built-in middleware, TLS, and security features.
package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/encryptcookie"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/proc"
	"github.com/natuleadan/sdk-api/runtime/errcode"
	"github.com/natuleadan/sdk-api/server/middleware"
	"github.com/samber/oops"
)

type RouteConfig struct {
	Path       string
	Middleware []string // middleware names: logger, shedding, breaker, maxconns, maxbytes, gunzip, prometheus, trace, cors, jwt, content_security, cryption
}

type Config struct {
	Port         int
	Host         string
	Prefork      bool
	BodyLimit    int
	Timeout      time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxConns     int
	MaxBytes     int

	MetricsPath      string
	HealthPath       string
	HealthEnabled    bool
	StartupPath      string
	ReadinessPath    string
	LivenessPath     string
	StartupEnabled   bool
	ReadinessEnabled bool
	LivenessEnabled  bool
	ShutdownTimeout  time.Duration
	RecoverStack     bool
	APIPrefix        string
	Routes           []RouteConfig

	Logger            bool
	LoadShedding      bool
	Breaker           bool
	Compression       bool
	StreamRequestBody bool
	ReduceMemoryUsage bool

	// Security
	SecurityHeaders *middleware.SecurityHeadersConfig
	CSRF            *middleware.CSRFConfig
	RateLimit       *middleware.RateLimitConfig
	TLS             *TLSConfig
	SSRF            *middleware.SSRFConfig
	// CSPGroups are named per-route CSP policies applied via middleware[].apply
	// "csp:<name>" or entry[].csp. Each group overrides the global CSP for its path.
	CSPGroups []CSPGroup
	// Correlation enables the X-Correlation-ID tracking middleware.
	Correlation *CorrelationConfig

	// Logger config
	LogSkipPaths  []string
	LogSampleRate float64
}

type CorrelationConfig struct {
	Enabled        bool
	RequestHeader  string
	ResponseHeader string
	SkipPaths      []string
}

type TelemetryConfig struct {
	Enabled  bool
	Name     string
	Endpoint string
	Sampler  float64
	Batcher  string

	OtlpHeaders         map[string]string
	OtlpHttpPath        string
	OtlpHttpSecure      bool
	TraceResponseHeader string
	SkipPaths           []string
}

type SecurityConfig struct {
	ContentSecurity *ContentSecurityConf
	Cryption        *CryptionConf
	EncryptCookie   *EncryptCookieConf
}

type CORSConfig struct {
	Origins             []string
	Methods             []string
	Headers             []string
	Credentials         bool
	MaxAge              int
	ExposeHeaders       []string
	AllowPrivateNetwork bool
	AllowOriginsFunc    func(origin string) bool
	Groups              []CORSGroup
}

type CORSGroup struct {
	Name                string
	PathPrefix          string
	Origins             []string
	Methods             []string
	Headers             []string
	Credentials         bool
	MaxAge              int
	ExposeHeaders       []string
	AllowPrivateNetwork bool
	AllowOriginsFunc    func(origin string) bool
}

type DocsCORSConfig struct {
	Origins     []string
	Methods     []string
	Headers     []string
	Credentials bool
	MaxAge      int
}

type ContentSecurityConf struct {
	Enabled   bool
	Strict    bool
	PublicKey string
}

// CSPGroup is a named per-route Content-Security-Policy. It overrides the
// global CSP for routes that reference it via middleware[].apply "csp:<name>"
// or entry[].csp. PathPrefix is resolved from those references.
type CSPGroup struct {
	Name       string
	PathPrefix string
	CSP        *middleware.CSPConfig
}

type CryptionConf struct {
	Enabled bool
	Key     string
}

type EncryptCookieConf struct {
	Enabled bool
	Key     string
	Except  []string
}

func DefaultConfig() Config {
	return Config{
		Port:              8080,
		Host:              "0.0.0.0",
		Prefork:           false,
		BodyLimit:         4 * 1024 * 1024,
		Timeout:           30 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxConns:          1000,
		MaxBytes:          4 << 20,
		MetricsPath:       "/metrics",
		HealthPath:        "/healthz",
		HealthEnabled:     true,
		StartupPath:       healthcheck.StartupEndpoint,
		ReadinessPath:     healthcheck.ReadinessEndpoint,
		LivenessPath:      healthcheck.LivenessEndpoint,
		StartupEnabled:    true,
		ReadinessEnabled:  true,
		LivenessEnabled:   true,
		ShutdownTimeout:   10 * time.Second,
		RecoverStack:      true,
		APIPrefix:         "/api",
		Logger:            true,
		LoadShedding:      true,
		Breaker:           true,
		LogSkipPaths:      []string{"/health", "/metrics"},
		LogSampleRate:     0,
		Compression:       false,
		StreamRequestBody: false,
		ReduceMemoryUsage: false,
	}
}

func Duration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

type Server struct {
	app    *fiber.App
	config Config

	// readinessProbe allows overriding the default readiness check. Nil keeps
	// the default (readyFlag). Set via SetReady.
	readinessProbe func() bool
	// readyFlag flips to true once startup tasks complete (MarkReady).
	readyFlag atomic.Bool
}

func New(cfg Config, telemetry TelemetryConfig, security SecurityConfig, corsCfg *CORSConfig) *Server {
	if cfg.Port == 0 {
		cfg = DefaultConfig()
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	app := fiber.New(fiber.Config{
		BodyLimit:         cfg.BodyLimit,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorHandler:      errorHandler,
		StreamRequestBody: cfg.StreamRequestBody,
		ReduceMemoryUsage: cfg.ReduceMemoryUsage,
	})

	s := &Server{app: app, config: cfg}
	setupGlobalMiddlewares(app, cfg, telemetry, s)
	setupSecurityMiddlewares(app, cfg, security)
	setupRouteOrGlobalMiddlewares(app, cfg, corsCfg)

	return s
}

func setupGlobalMiddlewares(app *fiber.App, cfg Config, telemetry TelemetryConfig, s *Server) {
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.RecoverStack}))
	app.Use(middleware.BodyReader())
	app.Use(middleware.HeaderSanitize())
	if cfg.Correlation != nil && cfg.Correlation.Enabled {
		app.Use(middleware.Correlation(middleware.CorrelationConfig{
			RequestHeader:  cfg.Correlation.RequestHeader,
			ResponseHeader: cfg.Correlation.ResponseHeader,
			SkipPaths:      cfg.Correlation.SkipPaths,
		}))
	}
	if cfg.HealthEnabled && cfg.HealthPath != "" {
		app.Get(cfg.HealthPath, healthcheck.New(healthcheck.Config{
			Probe: func(_ fiber.Ctx) bool { return true },
		}))
	}
	// 3-form health checks using Fiber's built-in healthcheck middleware.
	if cfg.StartupEnabled && cfg.StartupPath != "" {
		app.Get(cfg.StartupPath, healthcheck.New(healthcheck.Config{
			Probe: func(_ fiber.Ctx) bool { return s.startup() },
		}))
	}
	if cfg.ReadinessEnabled && cfg.ReadinessPath != "" {
		app.Get(cfg.ReadinessPath, healthcheck.New(healthcheck.Config{
			Probe: func(_ fiber.Ctx) bool { return s.ready() },
		}))
	}
	if cfg.LivenessEnabled && cfg.LivenessPath != "" {
		app.Get(cfg.LivenessPath, healthcheck.New(healthcheck.Config{
			Probe: func(_ fiber.Ctx) bool { return s.liveness() },
		}))
	}
	if telemetry.Enabled {
		app.Use(middleware.Trace(middleware.TraceConfig{
			Name:                telemetry.Name,
			Endpoint:            telemetry.Endpoint,
			Sampler:             telemetry.Sampler,
			Batcher:             telemetry.Batcher,
			OtlpHeaders:         telemetry.OtlpHeaders,
			OtlpHttpPath:        telemetry.OtlpHttpPath,
			OtlpHttpSecure:      telemetry.OtlpHttpSecure,
			TraceResponseHeader: telemetry.TraceResponseHeader,
			SkipPaths:           telemetry.SkipPaths,
		}))
	}
	app.Get(cfg.MetricsPath, middleware.PrometheusHandler())
}

func setupSecurityMiddlewares(app *fiber.App, cfg Config, security SecurityConfig) {
	applySecurityHeaders(app, cfg)
	applyCSRF(app, cfg)
	applyRateLimit(app, cfg)
	applyContentSecurity(app, security)
	applyCryption(app, security)
	applyEncryptCookie(app, security)
}

func applySecurityHeaders(app *fiber.App, cfg Config) {
	if cfg.SecurityHeaders == nil {
		return
	}
	// Global security headers: skip paths owned by a csp_group so the
	// per-route CSP can override without being clobbered by the global.
	if len(cfg.CSPGroups) > 0 && cfg.SecurityHeaders.Next == nil {
		headers := *cfg.SecurityHeaders
		headers.Next = func(c fiber.Ctx) bool {
			path := c.Path()
			for _, g := range cfg.CSPGroups {
				if g.PathPrefix != "" && strings.HasPrefix(path, g.PathPrefix) {
					return true
				}
			}
			return false
		}
		app.Use(middleware.SecurityHeaders(headers))
	} else {
		app.Use(middleware.SecurityHeaders(*cfg.SecurityHeaders))
	}
	// Register per-route CSP groups as global middleware with Next skip.
	// This ensures entry routes (registered directly on app) inherit the CSP.
	for _, g := range cfg.CSPGroups {
		if g.PathPrefix == "" || g.CSP == nil {
			continue
		}
		groupCfg := *cfg.SecurityHeaders
		groupCfg.CSP = middleware.BuildCSP(*g.CSP)
		groupCfg.Next = func(c fiber.Ctx) bool {
			return !strings.HasPrefix(c.Path(), g.PathPrefix)
		}
		app.Use(middleware.SecurityHeaders(groupCfg))
	}
}

func applyCSRF(app *fiber.App, cfg Config) {
	if cfg.CSRF != nil {
		app.Use(middleware.CSRF(*cfg.CSRF))
	}
}

func applyRateLimit(app *fiber.App, cfg Config) {
	if cfg.RateLimit != nil {
		app.Use(middleware.RateLimit(*cfg.RateLimit))
	}
}

func applyContentSecurity(app *fiber.App, security SecurityConfig) {
	if security.ContentSecurity != nil && security.ContentSecurity.Enabled {
		if key, err := middleware.ParsePublicKey(security.ContentSecurity.PublicKey); err == nil {
			app.Use(middleware.ContentSecurity(key, security.ContentSecurity.Strict))
		}
	}
}

func applyCryption(app *fiber.App, security SecurityConfig) {
	if security.Cryption != nil && security.Cryption.Enabled {
		app.Use(middleware.Cryption([]byte(security.Cryption.Key)))
	}
}

func applyEncryptCookie(app *fiber.App, security SecurityConfig) {
	if security.EncryptCookie != nil && security.EncryptCookie.Enabled {
		app.Use(encryptcookie.New(encryptcookie.Config{
			Key:    security.EncryptCookie.Key,
			Except: security.EncryptCookie.Except,
		}))
	}
}

func setupRouteOrGlobalMiddlewares(app *fiber.App, cfg Config, corsCfg *CORSConfig) {
	// Register utility middlewares only when enabled in YAML.
	if cfg.Logger {
		app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			SkipPaths:  cfg.LogSkipPaths,
			SampleRate: cfg.LogSampleRate,
		}))
	}
	if cfg.LoadShedding {
		app.Use(middleware.Shedding())
	}
	if cfg.Breaker {
		app.Use(middleware.Breaker())
	}
	if cfg.Compression {
		app.Use(compress.New())
	}
	app.Use(middleware.MaxConns(cfg.MaxConns))
	app.Use(middleware.MaxBytes(cfg.MaxBytes))
	app.Use(middleware.Gunzip())
	app.Use(middleware.Prometheus())

	// Register CORS only when configured in YAML (cors or cors_groups).
	if corsCfg != nil {
		if len(corsCfg.Origins) > 0 || corsCfg.AllowOriginsFunc != nil {
			corsHandler := middleware.CORS(middleware.CORSConfig{
				AllowedOrigins:      joinOrStar(corsCfg.Origins),
				AllowedMethods:      joinOrDefault(corsCfg.Methods, "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
				AllowedHeaders:      joinOrDefault(corsCfg.Headers, "Origin,Content-Type,Accept,Authorization"),
				AllowCredentials:    corsCfg.Credentials,
				MaxAge:              corsCfg.MaxAge,
				ExposeHeaders:       joinOrEmpty(corsCfg.ExposeHeaders),
				AllowPrivateNetwork: corsCfg.AllowPrivateNetwork,
				AllowOriginsFunc:    corsCfg.AllowOriginsFunc,
				Next: func(c fiber.Ctx) bool {
					return corsPathSkipped(c, corsCfg)
				},
			})
			app.Use(corsHandler)
		}
		for _, g := range corsCfg.Groups {
			if g.Name == "" {
				continue
			}
			registerCORSGroup(app, g)
		}
	}

	// Register per-route middlewares only when middleware[] has entries in YAML.
	if len(cfg.Routes) > 0 {
		setupPerRouteMiddlewares(app, cfg, corsCfg)
	}
}

func setupPerRouteMiddlewares(app *fiber.App, cfg Config, corsCfg *CORSConfig) {
	for _, rc := range cfg.Routes {
		grp := app.Group(rc.Path)
		for _, mw := range rc.Middleware {
			applyMiddlewareByType(grp, mw, cfg, corsCfg)
		}
	}
}

// corsPathSkipped returns true when the request path belongs to a named
// CORS group (which has its own middleware) so the global CORS skips it.
func corsPathSkipped(c fiber.Ctx, corsCfg *CORSConfig) bool {
	for _, g := range corsCfg.Groups {
		if g.Name != "" && g.PathPrefix != "" && strings.HasPrefix(c.Path(), g.PathPrefix) {
			return true
		}
	}
	return false
}

// registerCORSGroup registers a named CORS group as global middleware with
// a Next function that only applies to paths matching the group's prefix.
// This ensures entry routes (registered directly on app) inherit the CORS.
func registerCORSGroup(app *fiber.App, g CORSGroup) {
	prefix := g.PathPrefix
	if prefix == "" {
		return
	}
	app.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins:      joinOrStar(g.Origins),
		AllowedMethods:      joinOrDefault(g.Methods, "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
		AllowedHeaders:      joinOrDefault(g.Headers, "Origin,Content-Type,Accept,Authorization"),
		AllowCredentials:    g.Credentials,
		MaxAge:              g.MaxAge,
		ExposeHeaders:       joinOrEmpty(g.ExposeHeaders),
		AllowPrivateNetwork: g.AllowPrivateNetwork,
		AllowOriginsFunc:    g.AllowOriginsFunc,
		Next: func(c fiber.Ctx) bool {
			return !strings.HasPrefix(c.Path(), prefix)
		},
	}))
}

func applyMiddlewareByType(grp fiber.Router, name string, cfg Config, corsCfg *CORSConfig) {
	switch name {
	case "logger":
		grp.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			SkipPaths:  cfg.LogSkipPaths,
			SampleRate: cfg.LogSampleRate,
		}))
	case "shedding":
		grp.Use(middleware.Shedding())
	case "breaker":
		grp.Use(middleware.Breaker())
	case "maxconns":
		grp.Use(middleware.MaxConns(cfg.MaxConns))
	case "maxbytes":
		grp.Use(middleware.MaxBytes(cfg.MaxBytes))
	case "gunzip":
		grp.Use(middleware.Gunzip())
	case "prometheus":
		grp.Use(middleware.Prometheus())
	case "cors":
		if corsCfg != nil {
			grp.Use(middleware.CORS(middleware.CORSConfig{
				AllowedOrigins:      joinOrStar(corsCfg.Origins),
				AllowedMethods:      joinOrDefault(corsCfg.Methods, "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
				AllowedHeaders:      joinOrDefault(corsCfg.Headers, "Origin,Content-Type,Accept,Authorization"),
				AllowCredentials:    corsCfg.Credentials,
				MaxAge:              corsCfg.MaxAge,
				ExposeHeaders:       joinOrEmpty(corsCfg.ExposeHeaders),
				AllowPrivateNetwork: corsCfg.AllowPrivateNetwork,
				AllowOriginsFunc:    corsCfg.AllowOriginsFunc,
			}))
		}
	default:
		// cors:<group> applies a named CORS group scoped to this route group
		if groupName, ok := strings.CutPrefix(name, "cors:"); ok && corsCfg != nil {
			if g, found := findCORSGroup(corsCfg.Groups, groupName); found {
				grp.Use(middleware.CORS(middleware.CORSConfig{
					AllowedOrigins:      joinOrStar(g.Origins),
					AllowedMethods:      joinOrDefault(g.Methods, "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
					AllowedHeaders:      joinOrDefault(g.Headers, "Origin,Content-Type,Accept,Authorization"),
					AllowCredentials:    g.Credentials,
					MaxAge:              g.MaxAge,
					ExposeHeaders:       joinOrEmpty(g.ExposeHeaders),
					AllowPrivateNetwork: g.AllowPrivateNetwork,
					AllowOriginsFunc:    g.AllowOriginsFunc,
				}))
			}
		}
	}
}

func findCORSGroup(groups []CORSGroup, name string) (CORSGroup, bool) {
	for _, g := range groups {
		if g.Name == name {
			return g, true
		}
	}
	return CORSGroup{}, false
}

func joinOrStar(items []string) string {
	if len(items) == 0 {
		return "*"
	}
	var joined strings.Builder
	for i, s := range items {
		if i > 0 {
			joined.WriteString(", ")
		}
		joined.WriteString(s)
	}
	return joined.String()
}

func joinOrDefault(items []string, def string) string {
	if len(items) == 0 {
		return def
	}
	var joined strings.Builder
	for i, s := range items {
		if i > 0 {
			joined.WriteString(", ")
		}
		joined.WriteString(strings.TrimSpace(s))
	}
	return joined.String()
}

func joinOrEmpty(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var joined strings.Builder
	for i, s := range items {
		if i > 0 {
			joined.WriteString(",")
		}
		joined.WriteString(s)
	}
	return joined.String()
}

func (s *Server) App() *fiber.App {
	return s.app
}

func (s *Server) Start() error {
	return s.listenTLS()
}

func (s *Server) Stop() {
	logx.Info("server shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()
	if err := s.app.ShutdownWithContext(ctx); err != nil {
		logx.Errorf("server shutdown error: %v", err)
	}
}

func (s *Server) registerShutdown() { //nolint:unused
	proc.AddShutdownListener(func() {
		s.Stop()
	})
}

type ErrorResponse struct {
	Code    int    `json:"code"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message"`
}

func oopsCodeToHTTP(codeStr string) int {
	switch codeStr {
	case errcode.ErrCodeNotFound:
		return fiber.StatusNotFound
	case errcode.ErrCodeValidation:
		return fiber.StatusBadRequest
	case errcode.ErrCodeUnauthorized:
		return fiber.StatusUnauthorized
	case errcode.ErrCodeForbidden:
		return fiber.StatusForbidden
	case errcode.ErrCodeRateLimited:
		return fiber.StatusTooManyRequests
	case errcode.ErrCodeTimeout:
		return fiber.StatusGatewayTimeout
	default:
		return fiber.StatusInternalServerError
	}
}

func errorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	errCode := errcode.ErrCodeInternal
	message := "internal server error"

	var fe *fiber.Error
	if errors.As(err, &fe) {
		code = fe.Code
		message = fe.Message
	}

	var oo oops.OopsError
	if errors.As(err, &oo) {
		if c := oo.Code(); c != nil {
			errCode = c.(string)
			if code >= 500 {
				code = oopsCodeToHTTP(errCode)
			}
		}
		if p := oo.Public(); p != "" && code < 500 {
			message = p
		}
		logx.Errorw("request error",
			logx.Field("error", fmt.Sprintf("%+v", err)),
		)
	} else if code >= 500 {
		logx.Errorf("internal error: %v", err)
	}

	if code >= 500 {
		message = "internal server error"
	}

	return c.Status(code).JSON(ErrorResponse{
		Code:    code,
		Error:   errCode,
		Message: message,
	})
}

// Health probes ----------------------------------------------------------------

func (s *Server) startup() bool {
	// The process is alive; startup is always true once the server is wired.
	return true
}

func (s *Server) ready() bool {
	// Custom probe takes precedence.
	if s.readinessProbe != nil {
		return s.readinessProbe()
	}
	// Returns true once MarkReady / SetReady has been called (startup tasks done).
	return s.readyFlag.Load()
}

func (s *Server) liveness() bool {
	// Liveness: the process is running.
	return true
}

// MarkReady signals that startup tasks are complete and the server is ready to
// accept traffic. Readiness probes will now report true.
func (s *Server) MarkReady() {
	s.readyFlag.Store(true)
}

// SetReady allows callers to override the readiness probe function. The
// provided function is called on every readiness request.
func (s *Server) SetReady(fn func() bool) {
	s.readinessProbe = fn
	if fn != nil && fn() {
		s.readyFlag.Store(true)
	}
}

// registerHealthProbe registers a Fiber handler on the given path if enabled.
