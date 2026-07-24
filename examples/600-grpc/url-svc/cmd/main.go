package main

import (
	"context"
	"log"
	"os"

	"600-grpc/url-svc/internal/handler"
	"600-grpc/url-svc/internal/models"

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

	runtime.MustRegister[models.Link](svc, "Link", "primary", "links", nil)

	svc.WithSeed(func(ctx context.Context, s *runtime.Service) error {
		pool := s.PoolPGTyped("primary")
		tbl, err := db.NewTable[models.Link](pool, "links")
		if err != nil {
			return err
		}
		if err := tbl.AutoInit(ctx); err != nil {
			return err
		}
		svcCtx = handler.NewServiceContext(s, tbl)
		return nil
	})

	svc.WithRest("createLink", func(c *runtime.RestCtx) error {
		return handler.CreateLink(svcCtx)(c)
	})
	svc.WithRest("expandLink", func(c *runtime.RestCtx) error {
		return handler.ExpandLink(svcCtx)(c)
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
