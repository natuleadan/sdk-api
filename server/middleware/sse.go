package middleware

import (
	"github.com/gofiber/fiber/v3"
)

func SSE() fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		return c.Next()
	}
}
