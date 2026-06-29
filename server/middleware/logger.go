package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func Logger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		logx.Infof("%s %s %d %s",
			c.Method(),
			c.Path(),
			c.Response().StatusCode(),
			duration,
		)
		return err
	}
}
