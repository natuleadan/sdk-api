package runtime

import (
	"context"

	"github.com/gofiber/fiber/v3"
)

type RestCtx struct {
	fc fiber.Ctx
}

func newRestCtx(c fiber.Ctx) *RestCtx {
	return &RestCtx{fc: c}
}

func (c *RestCtx) Body() []byte {
	return c.fc.Body()
}

func (c *RestCtx) Params(key string) string {
	return c.fc.Params(key)
}

func (c *RestCtx) Query(key string, defaultValue ...string) string {
	return c.fc.Query(key, defaultValue...)
}

func (c *RestCtx) JSON(data any) error {
	return c.fc.JSON(data)
}

func (c *RestCtx) Status(code int) *RestCtx {
	c.fc.Status(code)
	return c
}

func (c *RestCtx) SendStatus(code int) error {
	return c.fc.SendStatus(code)
}

func (c *RestCtx) Context() context.Context {
	return c.fc.Context()
}

func (c *RestCtx) Method() string {
	return c.fc.Method()
}

func (c *RestCtx) Locals(key any, values ...any) any {
	return c.fc.Locals(key, values...)
}

func (c *RestCtx) SendString(s string) error {
	return c.fc.SendString(s)
}

func (c *RestCtx) Get(key string) string {
	return c.fc.Get(key)
}

func (c *RestCtx) Set(key, val string) {
	c.fc.Set(key, val)
}

func (c *RestCtx) Bind(v any) error {
	return c.fc.Bind().Body(v)
}

func (c *RestCtx) StatusCode() int {
	return c.fc.Response().StatusCode()
}

func (c *RestCtx) Path() string {
	return c.fc.Path()
}

func (c *RestCtx) ResponseBody() string {
	return string(c.fc.Response().Body())
}

func (c *RestCtx) SetCookie(cookie *fiber.Cookie) {
	c.fc.Cookie(cookie)
}

func (c *RestCtx) Redirect(url string, statusCode ...int) error {
	code := fiber.StatusFound
	if len(statusCode) > 0 {
		code = statusCode[0]
	}
	return c.fc.Redirect().Status(code).To(url)
}
