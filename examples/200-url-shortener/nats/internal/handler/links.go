package handler

import (
	"encoding/json"

	"url-shortener-nats/internal/svc"
	"url-shortener-nats/models"

	"github.com/natuleadan/sdk-api/runtime"
)

func RegisterRoutes(s *runtime.Service, svcCtx *svc.ServiceContext) {
	s.WithRest("getLinkById", getLinkById(svcCtx))
	s.WithRest("expandByShortCode", expandByShortCode(svcCtx))
	s.WithRest("natsRPC", natsRPC(svcCtx))
	s.WithRest("kvGet", kvGet(svcCtx))
	s.WithRest("kvSet", kvSet(svcCtx))
	s.WithRest("pullPublish", pullPublish(svcCtx))
	s.WithRest("pullMessages", pullMessages(svcCtx))
	s.WithRest("getEvents", getEvents(svcCtx))
	s.WithRest("clearEvents", clearEvents(svcCtx))
	s.WithRest("publishBulk", publishBulk(svcCtx))
}

func getLinkById(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		id := c.Params("id")
		ctx := c.Context()

		if conn := svcCtx.CacheConn(); conn != nil {
			val, err := conn.KVGet("url-expand-cache", "id."+id)
			if err == nil {
				var link models.Link
				if json.Unmarshal(val, &link) == nil {
					return c.JSON(link)
				}
			}
		}

		table := svcCtx.GetTable()
		link, err := table.Get(ctx, id)
		if err != nil {
			if err == runtime.ErrNotFound {
				return c.Status(404).JSON(map[string]any{"code": 404, "message": "not found"})
			}
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}

		if conn := svcCtx.CacheConn(); conn != nil {
			data, _ := json.Marshal(link)
			conn.KVPut("url-expand-cache", "id."+id, data)
		}

		return c.JSON(link)
	}
}

func expandByShortCode(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		shortCode := c.Params("shortCode")
		ctx := c.Context()

		if conn := svcCtx.CacheConn(); conn != nil {
			val, err := conn.KVGet("url-expand-cache", "sc."+shortCode)
			if err == nil {
				var link models.Link
				if json.Unmarshal(val, &link) == nil {
					return c.JSON(map[string]any{"targetUrl": link.TargetURL})
				}
			}
		}

		table := svcCtx.GetTable()
		link, err := table.FindBy(ctx, "short_code", shortCode)
		if err != nil {
			if err == runtime.ErrNotFound {
				return c.Status(404).JSON(map[string]any{"code": 404, "message": "not found"})
			}
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}

		if conn := svcCtx.CacheConn(); conn != nil {
			data, _ := json.Marshal(link)
			conn.KVPut("url-expand-cache", "sc."+shortCode, data)
		}

		svcCtx.PublishEvent(ctx, models.URLEvent{
			Type: "expanded", LinkID: link.ID,
			ShortCode: link.ShortCode, TargetURL: link.TargetURL,
		})

		return c.JSON(map[string]any{"targetUrl": link.TargetURL})
	}
}
