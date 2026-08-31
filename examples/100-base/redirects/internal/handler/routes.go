package handler

import (
	"fmt"

	"github.com/natuleadan/sdk-api/runtime"
)

func RegisterRoutes(s *runtime.Service) {
	s.WithRest("echoNew", echo("new"))
	s.WithRest("echoPosts", echo("posts"))
	s.WithRest("echoWildcard", func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"handler": "wildcard", "path": c.Path()})
	})
	s.WithRest("echoProfile", func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"handler": "profile", "id": c.Params("id")})
	})
	s.WithRest("echoSearch", func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"handler": "search", "query": c.Query("q")})
	})
	s.WithRest("echoV2", echo("v2"))
	s.WithRest("echoLimited", echo("limited"))
}

func echo(name string) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.SendString(fmt.Sprintf("handler:%s", name))
	}
}
