package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type CorrelationConfig struct {
	RequestHeader  string
	ResponseHeader string
	ContextKey     string
	SkipPaths      []string
}

func DefaultCorrelationConfig() CorrelationConfig {
	return CorrelationConfig{
		RequestHeader:  "X-Correlation-ID",
		ResponseHeader: "X-Correlation-ID",
		ContextKey:     "correlation_id",
	}
}

func Correlation(cfg CorrelationConfig) fiber.Handler {
	if cfg.RequestHeader == "" {
		cfg.RequestHeader = "X-Correlation-ID"
	}
	if cfg.ResponseHeader == "" {
		cfg.ResponseHeader = "X-Correlation-ID"
	}
	if cfg.ContextKey == "" {
		cfg.ContextKey = "correlation_id"
	}

	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		if _, skip := skipSet[string(c.Request().URI().Path())]; skip {
			return c.Next()
		}

		id := string(c.Request().Header.Peek(cfg.RequestHeader))
		if id == "" {
			id = uuid.New().String()
		}

		c.Set(cfg.ResponseHeader, id)
		c.Locals(cfg.ContextKey, id)

		return c.Next()
	}
}

func GetCorrelationID(c fiber.Ctx) string {
	if id, ok := c.Locals("correlation_id").(string); ok {
		return id
	}
	return ""
}
