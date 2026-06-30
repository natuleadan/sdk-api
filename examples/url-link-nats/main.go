package main

import (
	"log"
	"os"
	"time"

	"url-link-nats/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/runtime"
)

var linkCache *events.Cache[models.Link]

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	svc, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	pgPool := svc.Pool("pg-main").(*pgxpool.Pool)
	table, err := db.NewTable[models.Link](pgPool, "link")
	if err != nil {
		log.Fatalf("table: %v", err)
	}

	provider := runtime.NewCRUDProvider(table, &LinkHooks{})
	svc.WithCRUD("Link", provider)

	svc.WithRest("expandLink", func(c *fiber.Ctx) error {
		code := c.Params("shortCode")
		ctx := c.UserContext()
		cache := getCache(svc)

		if cache != nil {
			link, err := cache.Get(ctx, "sc."+code)
			if err == nil && link != nil {
				return c.JSON(fiber.Map{"url": link.TargetURL})
			}
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
