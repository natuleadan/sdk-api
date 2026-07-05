package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

type CORSConfig struct {
	AllowedOrigins   string
	AllowedMethods   string
	AllowedHeaders   string
	AllowCredentials bool
	MaxAge           int
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowedHeaders:   "Origin,Content-Type,Accept,Authorization",
		AllowCredentials: false,
		MaxAge:           300,
	}
}

func CORS(cfg CORSConfig) fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins:     strings.Split(cfg.AllowedOrigins, ","),
		AllowMethods:     strings.Split(cfg.AllowedMethods, ","),
		AllowHeaders:     strings.Split(cfg.AllowedHeaders, ","),
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           cfg.MaxAge,
	})
}
