package middleware

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
)

type DeprecationConfig struct {
	Status     string // "current", "deprecated", "removed"
	SunsetDate time.Time
	Message    string
}

func Deprecation(cfg DeprecationConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		switch cfg.Status {
		case "deprecated":
			c.Response().Header.Set("Deprecation", "true")
			if !cfg.SunsetDate.IsZero() {
				c.Response().Header.Set("Sunset", cfg.SunsetDate.Format(http.TimeFormat))
			}
			if cfg.Message != "" {
				c.Response().Header.Set("Deprecation-Message", cfg.Message)
			}

		case "removed":
			c.Response().Header.Set("Deprecation", "true")
			if !cfg.SunsetDate.IsZero() {
				c.Response().Header.Set("Sunset", cfg.SunsetDate.Format(http.TimeFormat))
			}
			if cfg.Message != "" {
				c.Response().Header.Set("Deprecation-Message", cfg.Message)
			}
			return c.Status(fiber.StatusGone).JSON(fiber.Map{
				"code":    410,
				"error":   "ERR_GONE",
				"message": "this endpoint has been removed",
			})
		}

		return c.Next()
	}
}
