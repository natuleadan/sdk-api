package middleware

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/gofiber/fiber/v2"
)

const gzipMagic = "\x1f\x8b\x08"

func Gunzip() fiber.Handler {
	return func(c *fiber.Ctx) error {
		body := c.Body()
		if len(body) < 3 || string(body[:3]) != gzipMagic {
			return c.Next()
		}

		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err == nil {
			defer reader.Close()
			if decoded, err := io.ReadAll(reader); err == nil {
				c.Request().SetBody(decoded)
			}
		}

		return c.Next()
	}
}
