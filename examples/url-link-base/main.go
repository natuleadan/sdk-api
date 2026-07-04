package main

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"url-link-base/models"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/redis/go-redis/v9"
)

var (
	rdb      *redis.Client
	rdbOnce  sync.Once
)

type linkCRUD struct {
	svc   *runtime.Service
	inner runtime.CRUDProvider
	once  sync.Once
}

func (l *linkCRUD) init() runtime.CRUDProvider {
	l.once.Do(func() {
		pgPool := l.svc.Pool("pg-main").(*pgxpool.Pool)
		table, err := db.NewTable[models.Link](pgPool, "link")
		if err != nil {
			log.Fatalf("table: %v", err)
		}
		if err := table.AutoInit(context.Background()); err != nil {
			log.Fatalf("autoinit: %v", err)
		}
		l.inner = runtime.NewCRUDProvider(table, &LinkHooks{})
	})
	return l.inner
}

func (l *linkCRUD) List(c *fiber.Ctx, params runtime.ListParams) error {
	return l.init().List(c, params)
}
func (l *linkCRUD) Get(c *fiber.Ctx, id string) error {
	return l.init().Get(c, id)
}
func (l *linkCRUD) Create(c *fiber.Ctx, body []byte) error {
	return l.init().Create(c, body)
}
func (l *linkCRUD) Update(c *fiber.Ctx, id string, body []byte) error {
	return l.init().Update(c, id, body)
}
func (l *linkCRUD) Delete(c *fiber.Ctx, id string) error {
	return l.init().Delete(c, id)
}

func getRedisAddr() string {
	if a := os.Getenv("REDIS_ADDR"); a != "" {
		return a
	}
	return "localhost:16379"
}

func getRedis() *redis.Client {
	rdbOnce.Do(func() {
		rdb = redis.NewClient(&redis.Options{
			Addr:         getRedisAddr(),
			PoolSize:     100,
			MinIdleConns: 10,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Printf("redis ping: %v", err)
			rdb = nil
		} else {
			log.Println("redis ready")
		}
	})
	return rdb
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

	svc.WithRest("expandLink", func(c *fiber.Ctx) error {
		code := c.Params("shortCode")
		ctx := c.UserContext()
		cache := getRedis()

		if cache != nil {
			url, err := cache.Get(ctx, "sc:"+code).Result()
			if err == nil {
				return c.JSON(fiber.Map{"url": url})
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
			cache.Set(ctx, "sc:"+code, link.TargetURL, 5*time.Minute)
		}

		return c.JSON(fiber.Map{"url": link.TargetURL})
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
