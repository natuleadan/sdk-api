package handler

import (
	"encoding/json"
	"fmt"

	"url-shortener-nats/internal/svc"
	"url-shortener-nats/models"

	"github.com/natuleadan/sdk-api/runtime"
)

func natsRPC(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		eventSvc := svcCtx.EventSvc()
		if eventSvc == nil {
			return c.Status(503).JSON(map[string]any{"code": 503, "message": "event broker not ready"})
		}
		data := c.Body()
		if len(data) == 0 {
			data = []byte("ping")
		}
		reply, err := eventSvc.Request(c.Context(), "nats-rpc.request", data, 10e9)
		if err != nil {
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}
		return c.SendString(string(reply))
	}
}

func kvGet(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		conn := svcCtx.CacheConn()
		if conn == nil {
			return c.Status(400).JSON(map[string]any{"code": 400, "message": "NATS KV requires NATS broker"})
		}
		val, err := conn.KVGet("demo-kv", c.Params("key"))
		if err != nil {
			return c.Status(404).JSON(map[string]any{"code": 404, "message": "key not found"})
		}
		return c.JSON(map[string]any{"key": c.Params("key"), "value": string(val)})
	}
}

func kvSet(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		conn := svcCtx.CacheConn()
		if conn == nil {
			return c.Status(400).JSON(map[string]any{"code": 400, "message": "NATS KV requires NATS broker"})
		}
		rev, err := conn.KVPut("demo-kv", c.Params("key"), c.Body())
		if err != nil {
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}
		return c.JSON(map[string]any{"key": c.Params("key"), "revision": rev})
	}
}

func pullPublish(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		eventSvc := svcCtx.EventSvc()
		if eventSvc == nil {
			return c.Status(503).JSON(map[string]any{"code": 503, "message": "event broker not ready"})
		}
		if err := eventSvc.Publish(c.Context(), "nats-pull.request", c.Body()); err != nil {
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}
		return c.SendStatus(202)
	}
}

func pullMessages(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		svcCtx.RLock()
		defer svcCtx.RUnlock()
		return c.JSON(svcCtx.PullMessages)
	}
}

func getEvents(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		svcCtx.EventRLock()
		defer svcCtx.EventRUnlock()
		return c.JSON(svcCtx.EventList)
	}
}

func clearEvents(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		svcCtx.EventLock()
		svcCtx.EventList = nil
		svcCtx.EventUnlock()
		return c.SendStatus(204)
	}
}

func publishBulk(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		eventSvc := svcCtx.EventSvc()
		if eventSvc == nil {
			return c.Status(503).JSON(map[string]any{"code": 503, "message": "event broker not ready"})
		}
		var req struct {
			Count   int    `json:"count"`
			Subject string `json:"subject"`
		}
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(400).JSON(map[string]any{"code": 400, "message": err.Error()})
		}
		if req.Count <= 0 || req.Count > 9999 {
			return c.Status(400).JSON(map[string]any{"code": 400, "message": "count must be 1-9999"})
		}
		if req.Subject == "" {
			req.Subject = "links.bulk"
		}
		for i := range req.Count {
			if err := eventSvc.PublishJSON(c.Context(), req.Subject, models.URLEvent{
				Type: "bulk", LinkID: int64(i),
				ShortCode: fmt.Sprintf("bulk%05d", i),
			}); err != nil {
				return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
			}
		}
		return c.JSON(map[string]any{"published": req.Count})
	}
}
