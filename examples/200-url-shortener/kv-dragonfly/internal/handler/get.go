package handler

import (
	"kv-dragonfly-v2/internal/logic"
	"kv-dragonfly-v2/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func getLink(svcCtx *svc.ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		id := c.Params("id")
		l := logic.NewLinkLogic(svcCtx.Redis)
		d, err := l.GetByID(c.Context(), id)
		if err != nil {
			return c.Status(404).JSON(runtime.Map{"error": "not found"})
		}
		return c.JSON(d)
	}
}
