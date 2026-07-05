package main

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"url-link-nats/models"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/runtime"
)

var linkCache *events.Cache[models.Link]

type linkCRUD struct {
	svc   *runtime.Service
	inner runtime.CRUDProvider
	once  sync.Once
	table *db.Table[models.Link]
}

func (l *linkCRUD) init() runtime.CRUDProvider {
	l.once.Do(func() {
		pgPool := l.svc.Pool("pg-main").(*pgxpool.Pool)
		var err error
		l.table, err = db.NewTable[models.Link](pgPool, "link")
		if err != nil {
			log.Fatalf("table: %v", err)
		}
		if err := l.table.AutoInit(context.Background()); err != nil {
			log.Fatalf("autoinit: %v", err)
		}
		l.inner = runtime.NewCRUDProvider(l.table, &LinkHooks{})
	})
	return l.inner
}

func (l *linkCRUD) List(c fiber.Ctx, params runtime.ListParams) error {
	return l.init().List(c, params)
}
func (l *linkCRUD) Get(c fiber.Ctx, id string) error {
	return l.init().Get(c, id)
}
func (l *linkCRUD) Create(c fiber.Ctx, body []byte) error {
	return l.init().Create(c, body)
}
func (l *linkCRUD) Update(c fiber.Ctx, id string, body []byte) error {
	return l.init().Update(c, id, body)
}
func (l *linkCRUD) Delete(c fiber.Ctx, id string) error {
	return l.init().Delete(c, id)
}

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	svc, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	provider := &linkCRUD{svc: svc}
	svc.WithCRUD("Link", provider)

	svc.WithRest("expandLink", func(c fiber.Ctx) error {
		code := c.Params("shortCode")
		ctx := c.Context()
		cache := getCache(svc)

		if cache != nil {
			link, err := cache.Get(ctx, "sc."+code)
			if err == nil && link != nil {
				return c.JSON(fiber.Map{"url": link.TargetURL})
			}
		}

		pgPool := svc.Pool("pg-main").(*pgxpool.Pool)
		table, err := db.NewTable[models.Link](pgPool, "link")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
		}
		link, err := table.FindBy(ctx, "short_code", code)
		if err != nil {
			if err == db.ErrNotFound {
				return c.Status(404).JSON(fiber.Map{"code": 404, "message": "link not found"})
			}
			return c.Status(500).JSON(fiber.Map{"code": 500, "message": err.Error()})
		}

		if cache != nil {
			cache.Set(ctx, "sc."+code, *link)
		}

		return c.JSON(fiber.Map{"url": link.TargetURL})
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func getCache(svc *runtime.Service) *events.Cache[models.Link] {
	if linkCache == nil {
		natsConn := svc.NATS("primary")
		if natsConn == nil {
			log.Println("nats not available")
			return nil
		}
		c, ok := natsConn.(*events.Conn)
		if !ok {
			log.Println("expected NATS broker")
			return nil
		}
		kv, err := c.EnsureKeyValue(events.DefaultKVConfig("url-link-cache"))
		if err != nil {
			log.Printf("cache init: %v", err)
			return nil
		}
		linkCache = events.NewCache[models.Link](kv, 30*time.Minute)
		log.Println("nats kv cache ready")
	}
	return linkCache
}

// fiber:context-methods migrated
