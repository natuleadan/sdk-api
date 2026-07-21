package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
)

func Ping() func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.SendString("pong")
	}
}
