package middleware

import (
	"bytes"

	"github.com/gofiber/fiber/v3"
)

func HeaderSanitize() fiber.Handler {
	return func(c fiber.Ctx) error {
		var hasInvalid bool
		for _, value := range c.Request().Header.All() {
			if containsCRLF(value) {
				hasInvalid = true
			}
		}
		if hasInvalid {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"code":    400,
				"message": "invalid header value",
			})
		}
		return c.Next()
	}
}

func containsCRLF(b []byte) bool {
	return bytes.IndexByte(b, '\r') >= 0 || bytes.IndexByte(b, '\n') >= 0
}
