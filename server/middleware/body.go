package middleware

import (
	"github.com/gofiber/fiber/v3"
)

const requestBodyKey = "request_body"

// BodyReader reads the request body once and caches it in context locals.
// Downstream middlewares should call getRequestBody() instead of c.Body().
// Must be registered before any middleware that reads the request body.
func BodyReader() fiber.Handler {
	return func(c fiber.Ctx) error {
		body := c.Body()
		c.Locals(requestBodyKey, body)
		return c.Next()
	}
}

// getRequestBody returns the cached request body from BodyReader,
// falling back to c.Body() if BodyReader has not been registered.
func getRequestBody(c fiber.Ctx) []byte {
	if body, ok := c.Locals(requestBodyKey).([]byte); ok {
		return body
	}
	return c.Body()
}

// setRequestBody updates the cached request body and the underlying
// request buffer. Must be called by middlewares that transform the body
// (e.g. Gunzip, Cryption).
func setRequestBody(c fiber.Ctx, body []byte) {
	c.Locals(requestBodyKey, body)
	c.Request().SetBody(body)
}
