package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

type CORSConfig struct {
	AllowedOrigins      string
	AllowedMethods      string
	AllowedHeaders      string
	AllowCredentials    bool
	MaxAge              int
	ExposeHeaders       string
	AllowPrivateNetwork bool
	AllowOriginsFunc    func(origin string) bool
	Next                func(fiber.Ctx) bool
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins:      "*",
		AllowedMethods:      "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowedHeaders:      "Origin,Content-Type,Accept,Authorization",
		AllowCredentials:    false,
		MaxAge:              300,
		ExposeHeaders:       "",
		AllowPrivateNetwork: false,
		AllowOriginsFunc:    nil,
		Next:                nil,
	}
}

func CORS(cfg CORSConfig) fiber.Handler {
	origins := splitAndTrim(cfg.AllowedOrigins)
	methods := splitAndTrim(cfg.AllowedMethods)
	headers := splitAndTrim(cfg.AllowedHeaders)
	exposeHeaders := []string{}
	if cfg.ExposeHeaders != "" {
		parts := strings.SplitSeq(cfg.ExposeHeaders, ",")
		for p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				exposeHeaders = append(exposeHeaders, trimmed)
			}
		}
	}
	return cors.New(cors.Config{
		AllowOrigins:        origins,
		AllowMethods:        methods,
		AllowHeaders:        headers,
		AllowCredentials:    cfg.AllowCredentials,
		MaxAge:              cfg.MaxAge,
		ExposeHeaders:       exposeHeaders,
		AllowPrivateNetwork: cfg.AllowPrivateNetwork,
		AllowOriginsFunc:    cfg.AllowOriginsFunc,
		Next:                cfg.Next,
	})
}

func splitAndTrim(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
