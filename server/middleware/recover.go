package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func Recovery() fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				logx.Errorf("panic recovered: %v", r)
				c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"code":    500,
					"message": "internal server error",
				})
			}
		}()
		return c.Next()
	}
}
