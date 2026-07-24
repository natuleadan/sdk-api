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

	// Initialize tables via SDK (replaces raw DDL)
	orderTbl, err := db.NewTable[models.Order](pool, "orders")
	if err != nil {
		log.Fatalf("order table: %v", err)
	}
	if err := orderTbl.AutoInit(ctx); err != nil {
		log.Fatalf("order autoinit: %v", err)
	}
	svcCtx.SetOrderTable(orderTbl)

	ticketTbl, err := db.NewTable[models.Ticket](pool, "tickets")
	if err != nil {
		log.Fatalf("ticket table: %v", err)
	}
	if err := ticketTbl.AutoInit(ctx); err != nil {
		log.Fatalf("ticket autoinit: %v", err)
	}
	svcCtx.SetTicketTable(ticketTbl)

	// MustRegister creates a lazy CRUD factory for the /tickets entry
	runtime.MustRegister[models.Ticket](s, "Ticket", "pg-main", "tickets", nil)

	if err := svcCtx.SeedData(ctx); err != nil {
		log.Fatalf("seed: %v", err)
	}

	handler.RegisterRoutes(s, svcCtx)

	if err := s.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
