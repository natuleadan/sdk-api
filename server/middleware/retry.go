package middleware

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type RetryConfig struct {
	MaxRetries      int
	InitialInterval time.Duration
	MaxBackoff      time.Duration
	Multiplier      float64
}

const retryAttemptKey = "retry_attempt"

func Retry(cfg RetryConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		method := c.Method()
		if method != "GET" && method != "HEAD" && method != "PUT" &&
			method != "DELETE" && method != "OPTIONS" {
			return c.Next()
		}
		if c.Locals(retryAttemptKey) != nil {
			return c.Next()
		}

		body := c.Body()
		path := c.Path()

		var lastErr error
		for i := 0; i <= cfg.MaxRetries; i++ {
			if i > 0 {
				c.Request().SetBody(body)
			}

			c.Locals(retryAttemptKey, i+1)
			lastErr = c.Next()

			if lastErr == nil && c.Response().StatusCode() < 500 {
				return nil
			}
			if lastErr == nil && c.Response().StatusCode() >= 500 {
				lastErr = fiber.NewError(c.Response().StatusCode(), "retryable error")
			}

			if i < cfg.MaxRetries {
				wait := nextBackoffDuration(i, cfg.InitialInterval, cfg.MaxBackoff, cfg.Multiplier)
				logx.Errorf("retry %s %s attempt %d/%d waiting %v: %v",
					method, path, i+1, cfg.MaxRetries, wait, lastErr)

				time.Sleep(wait)
			}
		}

		return lastErr
	}
}

func nextBackoffDuration(attempt int, initial, max time.Duration, multiplier float64) time.Duration {
	d := float64(initial)
	for range attempt {
		d *= multiplier
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		if d >= float64(max) {
			return max
		}
		return time.Duration(d)
	}
	d += float64(n.Int64()) * float64(time.Millisecond)
	if d >= float64(max) {
		return max
	}
	return time.Duration(d)
}
