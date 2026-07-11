package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"url-shortener-nats/models"

	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/runtime"
)

var (
	eventSvc      events.EventBroker
	eventList     []models.URLEvent
	eventMu       sync.RWMutex
	pullMessages  [][]byte
	pullMu        sync.RWMutex
	svc           *runtime.Service
	cacheConn     *events.Conn
	cacheOnce     sync.Once
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	var err error
	svc, err = runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	runtime.MustRegister(svc, "Link", "pg-main", "link", &LinkHooks{codeByID: make(map[int64]string)})

	// Override CRUD Get with cache-aside for GET /links/:id
	svc.WithRest("getLinkById", func(c *runtime.RestCtx) error {
		id := c.Params("id")
		ctx := c.Context()

		// Cache-aside: check NATS KV first
		if conn := ensureCacheConn(); conn != nil {
			val, err := conn.KVGet("url-expand-cache", "id."+id)
			if err == nil {
				var link models.Link
				if json.Unmarshal(val, &link) == nil {
					return c.JSON(link)
				}
			}
		}

		// Miss — query PostgreSQL
		table := runtime.GetTable[models.Link](svc, "Link")
		link, err := table.Get(ctx, id)
		if err != nil {
			if err == runtime.ErrNotFound {
				return c.Status(404).JSON(map[string]any{"code": 404, "message": "not found"})
			}
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}

		// Populate cache on miss
		if conn := ensureCacheConn(); conn != nil {
			data, _ := json.Marshal(link)
			conn.KVPut("url-expand-cache", "id."+id, data)
		}

		return c.JSON(link)
	})

	svc.WithRest("expandByShortCode", func(c *runtime.RestCtx) error {
		shortCode := c.Params("shortCode")
		ctx := c.Context()

		// Cache-aside: check NATS KV first
		if conn := ensureCacheConn(); conn != nil {
			val, err := conn.KVGet("url-expand-cache", "sc."+shortCode)
			if err == nil {
				var link models.Link
				if json.Unmarshal(val, &link) == nil {
					return c.JSON(map[string]any{"targetUrl": link.TargetURL})
				}
			}
		}

		// Miss — query PostgreSQL
		table := runtime.GetTable[models.Link](svc, "Link")
		link, err := table.FindBy(ctx, "short_code", shortCode)
		if err != nil {
			if err == runtime.ErrNotFound {
				return c.Status(404).JSON(map[string]any{"code": 404, "message": "not found"})
			}
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}

		// Populate cache on miss
		if conn := ensureCacheConn(); conn != nil {
			data, _ := json.Marshal(link)
			conn.KVPut("url-expand-cache", "sc."+shortCode, data)
		}

		// Publish expand event
		ensureEventSvc()
		if eventSvc != nil {
			eventSvc.PublishJSON(ctx, "links.expanded", models.URLEvent{
				Type:      "expanded",
				LinkID:    link.ID,
				ShortCode: link.ShortCode,
				TargetURL: link.TargetURL,
			})
		}

		return c.JSON(map[string]any{"targetUrl": link.TargetURL})
	})

	svc.WithRest("natsRPC", func(c *runtime.RestCtx) error {
		ensureEventSvc()
		if eventSvc == nil {
			return c.Status(503).JSON(map[string]any{"code": 503, "message": "event broker not ready"})
		}
		data := c.Body()
		if len(data) == 0 {
			data = []byte("ping")
		}
		reply, err := eventSvc.Request(c.Context(), "nats-rpc.request", data, 10*time.Second)
		if err != nil {
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}
		return c.SendString(string(reply))
	})

	svc.WithRest("kvGet", func(c *runtime.RestCtx) error {
		conn := svc.NATS("primary")
		natsConn, ok := conn.(*events.Conn)
		if !ok {
			return c.Status(400).JSON(map[string]any{"code": 400, "message": "NATS KV requires NATS broker"})
		}
		val, err := natsConn.KVGet("demo-kv", c.Params("key"))
		if err != nil {
			return c.Status(404).JSON(map[string]any{"code": 404, "message": "key not found"})
		}
		return c.JSON(map[string]any{"key": c.Params("key"), "value": string(val)})
	})

	svc.WithRest("kvSet", func(c *runtime.RestCtx) error {
		conn := svc.NATS("primary")
		natsConn, ok := conn.(*events.Conn)
		if !ok {
			return c.Status(400).JSON(map[string]any{"code": 400, "message": "NATS KV requires NATS broker"})
		}
		rev, err := natsConn.KVPut("demo-kv", c.Params("key"), c.Body())
		if err != nil {
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}
		return c.JSON(map[string]any{"key": c.Params("key"), "revision": rev})
	})

	svc.WithRest("pullPublish", func(c *runtime.RestCtx) error {
		ensureEventSvc()
		if eventSvc == nil {
			return c.Status(503).JSON(map[string]any{"code": 503, "message": "event broker not ready"})
		}
		if err := eventSvc.Publish(c.Context(), "nats-pull.request", c.Body()); err != nil {
			return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
		}
		return c.SendStatus(202)
	})

	svc.WithRest("pullMessages", func(c *runtime.RestCtx) error {
		pullMu.RLock()
		defer pullMu.RUnlock()
		return c.JSON(pullMessages)
	})

	svc.WithRest("getEvents", func(c *runtime.RestCtx) error {
		eventMu.RLock()
		defer eventMu.RUnlock()
		return c.JSON(eventList)
	})

	svc.WithRest("clearEvents", func(c *runtime.RestCtx) error {
		eventMu.Lock()
		eventList = nil
		eventMu.Unlock()
		return c.SendStatus(204)
	})

	svc.WithRest("publishBulk", func(c *runtime.RestCtx) error {
		ensureEventSvc()
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
				Type:      "bulk",
				LinkID:    int64(i),
				ShortCode: fmt.Sprintf("bulk%05d", i),
			}); err != nil {
				return c.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
			}
		}
		return c.JSON(map[string]any{"published": req.Count})
	})

	svc.WithExit("onLinkEvent", onLinkEventHandler)
	svc.WithExit("onPullMsg", onPullMsgHandler)

	go startRPCCoreSub()

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func startRPCCoreSub() {
	for i := 0; i < 30; i++ {
		ensureEventSvc()
		if eventSvc != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	conn, ok := eventSvc.(*events.Conn)
	if !ok || conn == nil {
		log.Println("rpc: NATS broker not available")
		return
	}
	if err := conn.SubscribeRawReply("nats-rpc.request", func(data []byte) []byte {
		return data
	}); err != nil {
		log.Printf("rpc core sub: %v", err)
		return
	}
	log.Println("rpc handler ready (core NATS)")
}

func onLinkEventHandler(ctx context.Context, msg []byte) ([]byte, error) {
	var evt models.URLEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		return nil, err
	}
	eventMu.Lock()
	eventList = append(eventList, evt)
	eventMu.Unlock()
	return nil, nil
}

func onPullMsgHandler(ctx context.Context, msg []byte) ([]byte, error) {
	pullMu.Lock()
	pullMessages = append(pullMessages, msg)
	pullMu.Unlock()
	return nil, nil
}

func ensureEventSvc() {
	if eventSvc == nil && svc != nil {
		eventSvc = svc.NATS("primary")
	}
}

func ensureCacheConn() *events.Conn {
	cacheOnce.Do(func() {
		ensureEventSvc()
		conn, ok := eventSvc.(*events.Conn)
		if !ok || conn == nil {
			return
		}
		cacheConn = conn
	})
	return cacheConn
}
