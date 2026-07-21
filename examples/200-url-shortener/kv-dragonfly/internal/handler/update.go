package handler

import (
	"kv-dragonfly-v2/internal/logic"
	"kv-dragonfly-v2/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func updateLink(svcCtx *svc.ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		id := c.Params("id")
		var body logic.LinkBody
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		l := logic.NewLinkLogic(svcCtx.Redis)
		data, err := l.Update(c.Context(), id, body)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"error": err.Error()})
		}
		return c.JSON(data)
	}
}
