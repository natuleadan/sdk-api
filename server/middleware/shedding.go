package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/load"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type SheddingConfig struct {
	// Allow is a function that returns nil if the request is allowed.
	// If it returns an error, the request is rejected with 503.
	// If nil, the default adaptive CPU-based shedder is used.
	Allow func() error
}

var shedder = load.NewAdaptiveShedder()

func Shedding(cfg ...SheddingConfig) fiber.Handler {
	var (
		allow      func() error
		useDefault = true
	)
	if len(cfg) > 0 && cfg[0].Allow != nil {
		allow = cfg[0].Allow
		useDefault = false
	}

	return func(c fiber.Ctx) error {
		var err error
		if useDefault {
			var cb load.Promise
			cb, err = shedder.Allow()
			if err == nil {
				defer func() {
					if c.Response().StatusCode() >= 500 {
						cb.Fail()
					} else {
						cb.Pass()
					}
				}()
			}
		} else {
			err = allow()
		}

		if err != nil {
			logx.Errorf("shedding reject: %s %s", c.Method(), c.Path())
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"code":    503,
				"message": "server is overloaded",
			})
		}
		return c.Next()
	}
}
