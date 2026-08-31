package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
)

func RegisterRoutes(s *runtime.Service) {
	s.WithRest("healthz", func(c *runtime.RestCtx) error {
		return c.SendString("OK")
	})
}
