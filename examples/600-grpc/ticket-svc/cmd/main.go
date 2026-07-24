package main

import (
	"context"
	"log"
	"os"

	"600-grpc/ticket-svc/internal/handler"
	"600-grpc/ticket-svc/internal/models"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	svc, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svc.WithAuthValidator(func(ctx context.Context, a *middleware.AuthContext, roles, permissions []string) error {
		return nil
	})

	runtime.MustRegister[models.Ticket](svc, "Ticket", "primary", "tickets", nil)
	runtime.MustRegister[models.Order](svc, "Order", "primary", "orders", nil)

	var svcCtx *handler.ServiceContext

	svc.WithSeed(func(ctx context.Context, s *runtime.Service) error {
		pool := s.PoolPGTyped("primary")
		tbl, err := db.NewTable[models.Order](pool, "orders")
		if err != nil {
			return err
		}
		if err := tbl.AutoInit(ctx); err != nil {
			return err
		}
		svcCtx = handler.NewServiceContext(s, tbl)
		return nil
	})

	svc.WithRest("buyTicket", func(c *runtime.RestCtx) error {
		return handler.BuyTicket(svcCtx)(c)
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
