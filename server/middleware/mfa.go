package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

// MFARequired returns middleware that requires mfa: true in JWT claims.
func MFARequired() fiber.Handler {
	return func(c fiber.Ctx) error {
		raw := c.Locals("claims")
		claims, ok := raw.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "authentication required",
			})
		}
		mfa, ok := claims["mfa"].(bool)
		if !ok || !mfa {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "MFA verification required",
			})
		}
		return c.Next()
	}
}
