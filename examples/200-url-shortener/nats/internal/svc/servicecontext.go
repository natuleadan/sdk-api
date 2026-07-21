package svc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"url-shortener-nats/models"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/runtime"
)

type ServiceContext struct {
	svc          *runtime.Service
	eventSvc     events.EventBroker
	cacheConn    *events.Conn
	cacheOnce    sync.Once
	eventSvcOnce sync.Once
	EventList    []models.URLEvent
	eventMu      sync.RWMutex
	PullMessages [][]byte
	pullMu       sync.RWMutex
}

func NewServiceContext(s *runtime.Service) *ServiceContext {
	return &ServiceContext{svc: s}
}

func (c *ServiceContext) ensureEventSvc() {
	c.eventSvcOnce.Do(func() {
		if s := c.svc; s != nil {
			c.eventSvc = s.NATS("primary")
		}
	})
}

func (c *ServiceContext) ensureCacheConn() *events.Conn {
	c.cacheOnce.Do(func() {
		c.ensureEventSvc()
		conn, ok := c.eventSvc.(*events.Conn)
		if !ok || conn == nil {
			return
		}
		c.cacheConn = conn
	})
	return c.cacheConn
}

func (c *ServiceContext) EventSvc() events.EventBroker {
	c.ensureEventSvc()
	return c.eventSvc
}

func (c *ServiceContext) CacheConn() *events.Conn {
	return c.ensureCacheConn()
}

func (c *ServiceContext) StartRPCCoreSub() {
	for i := 0; i < 30; i++ {
		c.ensureEventSvc()
		if c.eventSvc != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	conn, ok := c.eventSvc.(*events.Conn)
	if !ok || conn == nil {
		log.Println("rpc: NATS broker not available")
		return
	}
	if err := conn.SubscribeRawReply("nats-rpc.request", func(data []byte) []byte {
		return data
	}); err != nil {
		logx.Errorf("rpc core sub: %v", err)
		return
	}
	log.Println("rpc handler ready (core NATS)")
}

func (c *ServiceContext) OnLinkEvent(ctx context.Context, msg []byte) ([]byte, error) {
	var evt models.URLEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		return nil, err
	}
	c.eventMu.Lock()
	c.EventList = append(c.EventList, evt)
	c.eventMu.Unlock()
	return nil, nil
}

func (c *ServiceContext) OnPullMsg(ctx context.Context, msg []byte) ([]byte, error) {
	c.pullMu.Lock()
	c.PullMessages = append(c.PullMessages, msg)
	c.pullMu.Unlock()
	return nil, nil
}

func (c *ServiceContext) GetTable() *db.Table[models.Link] {
	return runtime.GetTable[models.Link](c.svc, "Link")
}

func (c *ServiceContext) PublishEvent(ctx context.Context, evt models.URLEvent) error {
	c.ensureEventSvc()
	if c.eventSvc == nil {
		return nil
	}
	subject := "links." + evt.Type
	return c.eventSvc.PublishJSON(ctx, subject, evt)
}

func (c *ServiceContext) CacheDelete(idKey, scKey string) {
	if conn := c.ensureCacheConn(); conn != nil {
		if err := conn.KVDelete("url-expand-cache", idKey); err != nil {
			fmt.Printf("cache del %s: %v\n", idKey, err)
		}
		if scKey != "" {
			if err := conn.KVDelete("url-expand-cache", scKey); err != nil {
				fmt.Printf("cache del %s: %v\n", scKey, err)
			}
		}
	}
}

func (c *ServiceContext) RLock()    { c.pullMu.RLock() }
func (c *ServiceContext) RUnlock()  { c.pullMu.RUnlock() }
func (c *ServiceContext) EventRLock()   { c.eventMu.RLock() }
func (c *ServiceContext) EventRUnlock()  { c.eventMu.RUnlock() }
func (c *ServiceContext) EventLock()     { c.eventMu.Lock() }
func (c *ServiceContext) EventUnlock()   { c.eventMu.Unlock() }
