package handler

import (
	"kv-dragonfly-v2/internal/logic"
	"kv-dragonfly-v2/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func expandLink(svcCtx *svc.ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		code := c.Params("shortCode")
		l := logic.NewLinkLogic(svcCtx.Redis)
		d, err := l.GetByShortCode(c.Context(), code)
		if err != nil {
			return c.Status(404).JSON(runtime.Map{"error": "not found"})
		}
		return c.JSON(d)
	}
}
