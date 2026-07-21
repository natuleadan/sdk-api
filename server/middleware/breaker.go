package middleware

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/breaker"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type BreakerConfig struct {
	// OnRejected is called when the circuit breaker rejects a request.
	// If nil, a default 503 response is returned.
	OnRejected func(fiber.Ctx) error
}

func Breaker(cfg ...BreakerConfig) fiber.Handler {
	onRejected := defaultOnRejected
	if len(cfg) > 0 && cfg[0].OnRejected != nil {
		onRejected = cfg[0].OnRejected
	}

	return func(c fiber.Ctx) error {
		name := c.Method() + ":" + c.Route().Path
		b := breaker.GetBreaker(name)

		err := b.DoWithAcceptable(func() error {
			return c.Next()
		}, func(err error) bool {
			if err == nil {
				return true
			}
			// fiber.Error already has a status code (set before error handler runs)
			var fe *fiber.Error
			if errors.As(err, &fe) && fe.Code >= 400 && fe.Code < 500 {
				return true
			}
			return isClientError(c.Response().StatusCode())
		})

		if err == breaker.ErrServiceUnavailable {
			logx.Errorf("breaker open: %s", name)
			return onRejected(c)
		}
		return err
	}
}

func BreakerStates() fiber.Handler {
	return func(c fiber.Ctx) error {
		return c.JSON(breaker.States())
	}
}

func defaultOnRejected(c fiber.Ctx) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
		"code":    503,
		"message": "service temporarily unavailable",
	})
}

func isClientError(code int) bool {
	return code >= 400 && code < 500
}
