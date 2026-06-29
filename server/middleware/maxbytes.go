package middleware

import (
	"github.com/gofiber/fiber/v2"
)

func MaxBytes(limit int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if len(c.Body()) > limit {
			return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
				"code":    413,
				"message": "request body too large",
			})
		}
		return c.Next()
	}
}
