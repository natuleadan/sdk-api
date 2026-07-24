package main

import (
	"context"
	"log"
	"os"

	"600-grpc/transfer-svc/internal/handler"
	"600-grpc/transfer-svc/internal/models"

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

	var svcCtx *handler.ServiceContext

	svc.WithSeed(func(ctx context.Context, s *runtime.Service) error {
		pool := s.PoolPGTyped("primary")
		tbl, err := db.NewTable[models.Transfer](pool, "transfers")
		if err != nil {
			return err
		}
		if err := tbl.AutoInit(ctx); err != nil {
			return err
		}
		svcCtx = handler.NewServiceContext(s, tbl)
		return nil
	})

	svc.WithExit("onTransferCompleted", func(ctx context.Context, msg []byte) ([]byte, error) {
		return handler.OnTransferCompleted(svcCtx)(ctx, msg)
	})

	svc.WithRest("initiateTransfer", func(c *runtime.RestCtx) error {
		return handler.InitiateTransfer(svcCtx)(c)
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
