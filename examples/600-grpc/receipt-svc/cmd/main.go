package main

import (
	"context"
	"log"
	"os"

	"600-grpc/receipt-svc/internal/handler"

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

	svcCtx := handler.NewServiceContext()
	svc.WithRest("generateReceipt", handler.GenerateReceipt(svcCtx))

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
