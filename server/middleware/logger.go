package middleware

import (
	"github.com/gofiber/fiber/v3"
)

func Logger() fiber.Handler {
	return LoggerWithConfig(LoggerConfig{})
}
