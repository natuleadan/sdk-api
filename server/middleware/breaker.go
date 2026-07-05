package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/breaker"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func Breaker() fiber.Handler {
	return func(c fiber.Ctx) error {
		name := c.Method() + ":" + c.Route().Path
		b := breaker.NewBreaker(breaker.WithName(name))

		err := b.DoWithAcceptable(func() error {
			return c.Next()
		}, func(err error) bool {
			return err == nil || isClientError(c.Response().StatusCode())
		})

		if err == breaker.ErrServiceUnavailable {
			logx.Errorf("breaker open: %s", name)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"code":    503,
				"message": "service temporarily unavailable",
			})
		}
		return err
	}
}

func isClientError(code int) bool {
	return code >= 400 && code < 500
}
