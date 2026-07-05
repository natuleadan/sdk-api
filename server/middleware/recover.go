package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func Recovery() fiber.Handler {
	return func(c fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				logx.Errorf("panic recovered: %v", r)
				if err := c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"code":    500,
					"message": "internal server error",
				}); err != nil {
					logx.Errorf("recover: json error response failed: %v", err)
				}
			}
		}()
		return c.Next()
	}
}
