package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/load"
	"github.com/natuleadan/sdk-api/infra/logx"
)

var shedder = load.NewAdaptiveShedder()

func Shedding() fiber.Handler {
	return func(c *fiber.Ctx) error {
		cb, err := shedder.Allow()
		if err != nil {
			logx.Errorf("shedding reject: %s %s", c.Method(), c.Path())
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"code":    503,
				"message": "server is overloaded",
			})
		}
		defer func() {
			if c.Response().StatusCode() >= 500 {
				cb.Fail()
			} else {
				cb.Pass()
			}
		}()
		return c.Next()
	}
}
