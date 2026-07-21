package handler

import (
	"kv-dragonfly-v2/internal/logic"
	"kv-dragonfly-v2/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func listLinks(svcCtx *svc.ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		l := logic.NewLinkLogic(svcCtx.Redis)
		results, total, err := l.List(c.Context(), 1, 20)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"error": err.Error()})
		}
		return c.JSON(runtime.Map{
			"data":  results,
			"total": total,
			"page":  1,
			"size":  20,
		})
	}
}
