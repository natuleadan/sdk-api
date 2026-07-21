package middleware

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func Logger() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		corrID := GetCorrelationID(c)
		if corrID != "" {
			logx.Infof("[%s] %s %s %d %s",
				corrID,
				c.Method(),
				c.Path(),
				c.Response().StatusCode(),
				duration,
			)
		} else {
			logx.Infof("%s %s %d %s",
				c.Method(),
				c.Path(),
				c.Response().StatusCode(),
				duration,
			)
		}
		return err
	}
}
