package middleware

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/stores/cache"
)

type responseCacheWriter struct {
	fiber.Ctx
	body   bytes.Buffer
	status int
	cached bool
}

func (w *responseCacheWriter) Send(body []byte) error {
	if !w.cached {
		w.body.Write(body)
	}
	return w.Ctx.Send(body)
}

func (w *responseCacheWriter) Status(status int) fiber.Ctx {
	if !w.cached {
		w.status = status
	}
	return w.Ctx.Status(status)
}

// CacheResponse wraps a handler to cache its GET responses in KV.
// The cache key is method:path. On subsequent GETs, the cached response is returned.
// Use for read-only REST endpoints where data changes infrequently.
func CacheResponse(cc cache.Cache, ttl time.Duration) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Method() != fiber.MethodGet {
			return c.Next()
		}

		key := "rest:" + c.Method() + ":" + c.Path()

		var cached []byte
		if err := cc.GetCtx(c.Context(), key, &cached); err == nil {
			c.Response().SetBodyRaw(cached)
			c.Response().Header.SetContentType("application/json")
			return nil
		}

		w := &responseCacheWriter{Ctx: c, cached: false}
		if err := c.Next(); err != nil {
			return err
		}

		if w.status >= 200 && w.status < 300 {
			data, _ := json.Marshal(w.body.Bytes())
			if err := cc.SetWithExpireCtx(c.Context(), key, data, ttl); err != nil {
				return c.JSON(map[string]any{"error": "cache set failed"})
			}
		}

		return nil
	}
}
