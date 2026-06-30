package server

import (
	"context"
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

	// Global middlewares (always on: recover, health, trace, prometheus metrics)
	app.Use(recover.New(recover.Config{
		EnableStackTrace: cfg.RecoverStack,
	}))
	app.Use(healthcheck.New(healthcheck.Config{
		LivenessProbe:    func(c *fiber.Ctx) bool { return true },
		LivenessEndpoint: cfg.HealthPath,
	}))

	if telemetry.Enabled {
		app.Use(middleware.Trace(middleware.TraceConfig{
			Name:     telemetry.Name,
			Endpoint: telemetry.Endpoint,
			Sampler:  telemetry.Sampler,
			Batcher:  telemetry.Batcher,
		}))
	}

	app.Get(cfg.MetricsPath, middleware.PrometheusHandler())

	// Security headers middleware (always-on if configured)
	if cfg.SecurityHeaders != nil {
		app.Use(middleware.SecurityHeaders(*cfg.SecurityHeaders))
	}

	// CSRF middleware (global if enabled)
	if cfg.CSRF != nil {
		app.Use(middleware.CSRF(*cfg.CSRF))
	}

	// Rate limit middleware (global if configured)
	if cfg.RateLimit != nil {
		app.Use(middleware.RateLimit(*cfg.RateLimit))
	}

	// Security middlewares (global if enabled)
	if security.ContentSecurity != nil && security.ContentSecurity.Enabled {
		if key, err := middleware.ParsePublicKey(security.ContentSecurity.PublicKey); err == nil {
			app.Use(middleware.ContentSecurity(key, security.ContentSecurity.Strict))
		}
	}

	if security.Cryption != nil && security.Cryption.Enabled {
		app.Use(middleware.Cryption([]byte(security.Cryption.Key)))
	}

	// Per-route middlewares
	if len(cfg.Routes) > 0 {
		for _, rc := range cfg.Routes {
			grp := app.Group(rc.Path)
			for _, mw := range rc.Middleware {
				switch mw {
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
		}
	} else {
		// No routes specified: apply all middlewares globally (backwards compatible)
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

	s := &Server{app: app, config: cfg}
	s.registerShutdown()
	return s
}

func joinOrStar(items []string) string {
	if len(items) == 0 {
		return "*"
	}
	joined := ""
	for i, s := range items {
		if i > 0 {
			joined += ", "
		}
		joined += s
	}
	return joined
}

func joinOrDefault(items []string, def string) string {
	if len(items) == 0 {
		return def
	}
	joined := ""
	for i, s := range items {
		if i > 0 {
			joined += ", "
		}
		joined += s
	}
	return joined
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
	return c.Status(code).JSON(ErrorResponse{
		Code:    code,
		Message: err.Error(),
	})
}
