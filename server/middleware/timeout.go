package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func Timeout(d time.Duration) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), d)
		defer cancel()

		c.SetContext(ctx)

		err := c.Next()

		if ctx.Err() == context.DeadlineExceeded {
			logx.Errorf("request timeout: %s %s", c.Method(), c.Path())
			return c.Status(fiber.StatusRequestTimeout).JSON(fiber.Map{
				"code":    408,
				"message": "request timeout",
			})
		}

		return err
	}
}

// fiber:context-methods migrated
