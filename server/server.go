package server

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/healthcheck"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/proc"
	"github.com/natuleadan/sdk-api/server/middleware"
)

type RouteConfig struct {
	Path       string
	Middleware []string // middleware names: logger, shedding, breaker, maxconns, maxbytes, gunzip, prometheus, trace, cors, jwt, content_security, cryption
}

type Config struct {
	Port            int
	Host            string
	Prefork         bool
	BodyLimit       int
	Timeout         time.Duration
	MaxConns        int
	MaxBytes        int
	MetricsPath     string
	HealthPath      string
	ShutdownTimeout time.Duration
	RecoverStack    bool
	APIPrefix       string
	Routes          []RouteConfig

	// Security
	SecurityHeaders *middleware.SecurityHeadersConfig
	CSRF            *middleware.CSRFConfig
	RateLimit       *middleware.RateLimitConfig
	TLS             *TLSConfig
	SSRF            *middleware.SSRFConfig
}

type TelemetryConfig struct {
	Enabled  bool
	Name     string
	Endpoint string
	Sampler  float64
	Batcher  string
}

type SecurityConfig struct {
	ContentSecurity *ContentSecurityConf
	Cryption        *CryptionConf
}

type CORSConfig struct {
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

type CryptionConf struct {
	Enabled bool
	Key     string
}

func DefaultConfig() Config {
	return Config{
		Port:            8080,
		Host:            "0.0.0.0",
		Prefork:         false,
		BodyLimit:       4 * 1024 * 1024,
		Timeout:         30 * time.Second,
		MaxConns:        1000,
		MaxBytes:        4 << 20,
		MetricsPath:     "/metrics",
		HealthPath:      "/health",
		ShutdownTimeout: 10 * time.Second,
		RecoverStack:    true,
		APIPrefix:       "/api/v1",
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
}

func New(cfg Config, telemetry TelemetryConfig, security SecurityConfig, corsCfg *CORSConfig) *Server {
	if cfg.Port == 0 {
		cfg = DefaultConfig()
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	app := fiber.New(fiber.Config{
		Prefork:      cfg.Prefork,
		BodyLimit:    cfg.BodyLimit,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
		IdleTimeout:  cfg.Timeout,
		ErrorHandler: errorHandler,
	})

	setupGlobalMiddlewares(app, cfg, telemetry)
	setupSecurityMiddlewares(app, cfg, security)
	setupRouteOrGlobalMiddlewares(app, cfg, corsCfg)

	s := &Server{app: app, config: cfg}
	s.registerShutdown()
	return s
}

func setupGlobalMiddlewares(app *fiber.App, cfg Config, telemetry TelemetryConfig) {
	app.Use(recover.New(recover.Config{EnableStackTrace: cfg.RecoverStack}))
	app.Use(middleware.HeaderSanitize())
	app.Use(healthcheck.New(healthcheck.Config{
		LivenessProbe:    func(c *fiber.Ctx) bool { return true },
		LivenessEndpoint: cfg.HealthPath,
	}))
	if telemetry.Enabled {
		app.Use(middleware.Trace(middleware.TraceConfig{
			Name: telemetry.Name, Endpoint: telemetry.Endpoint,
			Sampler: telemetry.Sampler, Batcher: telemetry.Batcher,
		}))
	}
	app.Get(cfg.MetricsPath, middleware.PrometheusHandler())
}

func setupSecurityMiddlewares(app *fiber.App, cfg Config, security SecurityConfig) {
	if cfg.SecurityHeaders != nil {
		app.Use(middleware.SecurityHeaders(*cfg.SecurityHeaders))
	}
	if cfg.CSRF != nil {
		app.Use(middleware.CSRF(*cfg.CSRF))
	}
	if cfg.RateLimit != nil {
		app.Use(middleware.RateLimit(*cfg.RateLimit))
	}
	if security.ContentSecurity != nil && security.ContentSecurity.Enabled {
		if key, err := middleware.ParsePublicKey(security.ContentSecurity.PublicKey); err == nil {
			app.Use(middleware.ContentSecurity(key, security.ContentSecurity.Strict))
		}
	}
	if security.Cryption != nil && security.Cryption.Enabled {
		app.Use(middleware.Cryption([]byte(security.Cryption.Key)))
	}
}

func setupRouteOrGlobalMiddlewares(app *fiber.App, cfg Config, corsCfg *CORSConfig) {
	if len(cfg.Routes) > 0 {
		setupPerRouteMiddlewares(app, cfg, corsCfg)
	} else {
		setupGlobalStandardMiddlewares(app, cfg, corsCfg)
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

func setupGlobalStandardMiddlewares(app *fiber.App, cfg Config, corsCfg *CORSConfig) {
	app.Use(middleware.Logger())
	app.Use(middleware.Shedding())
	app.Use(middleware.Breaker())
	app.Use(middleware.MaxConns(cfg.MaxConns))
	app.Use(middleware.MaxBytes(cfg.MaxBytes))
	app.Use(middleware.Gunzip())
	app.Use(middleware.Prometheus())
	if corsCfg != nil {
		app.Use(middleware.CORS(middleware.CORSConfig{
			AllowedOrigins:   joinOrStar(corsCfg.Origins),
			AllowedMethods:   joinOrDefault(corsCfg.Methods, "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
			AllowedHeaders:   joinOrDefault(corsCfg.Headers, "Origin,Content-Type,Accept,Authorization"),
			AllowCredentials: corsCfg.Credentials,
			MaxAge:           corsCfg.MaxAge,
		}))
	}
}

func applyMiddlewareByType(grp fiber.Router, name string, cfg Config, corsCfg *CORSConfig) {
	switch name {
	case "logger":
		grp.Use(middleware.Logger())
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
				AllowedOrigins:   joinOrStar(corsCfg.Origins),
				AllowedMethods:   joinOrDefault(corsCfg.Methods, "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
				AllowedHeaders:   joinOrDefault(corsCfg.Headers, "Origin,Content-Type,Accept,Authorization"),
				AllowCredentials: corsCfg.Credentials,
				MaxAge:           corsCfg.MaxAge,
			}))
		}
	}
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

func (s *Server) registerShutdown() {
	proc.AddShutdownListener(func() {
		s.Stop()
	})
}

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	message := sanitizeErrorMessage(err.Error(), code)
	if code >= 500 {
		logx.Errorf("internal error: %v", err)
	}
	return c.Status(code).JSON(ErrorResponse{
		Code:    code,
		Message: message,
	})
}

func sanitizeErrorMessage(msg string, code int) string {
	if code < 500 {
		return msg
	}
	return "internal server error"
}
