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
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	s, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svcCtx := svc.NewServiceContext(s)

	runtime.MustRegister[models.Ticket](s, "Ticket", "pg-main", "tickets", nil)

	s.WithSeed(func(ctx context.Context, s *runtime.Service) error {
		pool := s.PoolPGTyped("pg-main")

		orderTbl, err := db.NewTable[models.Order](pool, "orders")
		if err != nil {
			return err
		}
		svcCtx.SetOrderTable(orderTbl)

		ticketTbl, err := db.NewTable[models.Ticket](pool, "tickets")
		if err != nil {
			return err
		}
		svcCtx.SetTicketTable(ticketTbl)

		return nil
	})

	handler.RegisterRoutes(s, svcCtx)

	if err := s.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
