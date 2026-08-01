package handler

import (
	"strconv"

	"kv-dragonfly-v2/internal/logic"
	"kv-dragonfly-v2/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

const (
	defaultPage = 1
	defaultSize = 10
	maxPageSize = 100
)

func listLinks(svcCtx *svc.ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		page, _ := strconv.Atoi(c.Query("page", "1"))
		size, _ := strconv.Atoi(c.Query("size", "10"))
		if page < defaultPage {
			page = defaultPage
		}
		if size < 1 {
			size = defaultSize
		}
		if size > maxPageSize {
			size = maxPageSize
		}

		l := logic.NewLinkLogic(svcCtx.Redis)
		results, total, err := l.List(c.Context(), page, size)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"error": err.Error()})
		}
		return c.JSON(runtime.Map{
			"data":  results,
			"total": total,
			"page":  page,
			"size":  size,
		})
	}
}
