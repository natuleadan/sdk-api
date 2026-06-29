package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func MaxConns(limit int) fiber.Handler {
	sem := make(chan struct{}, limit)
	return func(c *fiber.Ctx) error {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			return c.Next()
		default:
			logx.Errorf("maxconns limit reached: %d", limit)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"code":    503,
				"message": "too many connections",
			})
		}
	}
}
