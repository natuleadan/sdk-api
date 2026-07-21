package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type FallbackConfig struct {
	// Mode is "degraded" or "stale". Empty means fallback is disabled.
	Mode string
	// Message is the response body for degraded mode.
	Message string
}

type staleEntry struct {
	body      []byte
	status    int
	headers   map[string]string
	timestamp time.Time
}

const staleTTL = 30 * time.Second

var (
	staleCacheMu sync.RWMutex
	staleCache   = make(map[string]*staleEntry)
)

func Fallback(cfg FallbackConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()
		if err != nil {
			if cfg.Mode == "degraded" {
				msg := cfg.Message
				if msg == "" {
					msg = "service temporarily unavailable"
				}
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"code":    503,
					"message": msg,
				})
			}

			if cfg.Mode == "stale" {
				key := c.Method() + ":" + c.Path()
				staleCacheMu.RLock()
				entry, ok := staleCache[key]
				staleCacheMu.RUnlock()
				if ok && time.Since(entry.timestamp) < staleTTL {
					logx.Infof("stale response for %s %s", c.Method(), c.Path())
					for k, v := range entry.headers {
						c.Response().Header.Set(k, v)
					}
					return c.Status(entry.status).Send(entry.body)
				}
			}

			return err
		}

		if cfg.Mode == "stale" && c.Response().StatusCode() == fiber.StatusOK {
			key := c.Method() + ":" + c.Path()
			entry := &staleEntry{
				body:      c.Response().Body(),
				status:    c.Response().StatusCode(),
				headers:   make(map[string]string),
				timestamp: time.Now(),
			}
			for k, v := range c.Response().Header.All() {
				entry.headers[string(k)] = string(v)
			}
			staleCacheMu.Lock()
			staleCache[key] = entry
			staleCacheMu.Unlock()
		}

		return nil
	}
}
