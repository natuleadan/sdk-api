package main

import (
	"context"
	"log"
	"os"

	"tickets/internal/handler"
	"tickets/internal/svc"
	"tickets/models"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

func main() {
	ctx := context.Background()

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	s, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	poolURL := os.Getenv("DATABASE_URL")
	if poolURL == "" {
		poolURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}
	pool, err := db.NewPool(ctx, db.PoolConfig{URL: poolURL})
	if err != nil {
		log.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	svcCtx := svc.NewServiceContext(s, pool)

	if err := svcCtx.EnsureTables(ctx); err != nil {
		log.Fatalf("tables: %v", err)
	}
	if err := svcCtx.SeedData(ctx); err != nil {
		log.Fatalf("seed: %v", err)
	}

	runtime.MustRegister[models.Ticket](s, "Ticket", "pg-main", "tickets", nil)

	handler.RegisterRoutes(s, svcCtx)

	if err := s.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
