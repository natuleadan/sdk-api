package handler

import (
	"kv-dragonfly-v2/internal/logic"
	"kv-dragonfly-v2/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func deleteLink(svcCtx *svc.ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		code := c.Params("id")
		l := logic.NewLinkLogic(svcCtx.Redis)
		if err := l.Delete(c.Context(), code); err != nil {
			return c.Status(404).JSON(runtime.Map{"error": "not found"})
		}
		return c.SendStatus(204)
	}
}
