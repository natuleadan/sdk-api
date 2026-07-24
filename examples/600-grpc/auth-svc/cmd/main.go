package main

import (
	"context"
	"log"
	"os"

	"600-grpc/auth-svc/internal/handler"
	"600-grpc/auth-svc/internal/models"
	"600-grpc/auth-svc/internal/server"
	authpb "600-grpc/pb/authpb"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
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

	svcCtx := handler.NewServiceContext()

	svc.WithAuthValidator(func(ctx context.Context, a *middleware.AuthContext, roles, permissions []string) error {
		return nil
	})

	runtime.MustRegister[models.User](svc, "User", "primary", "users", nil)

	svc.WithSeed(func(ctx context.Context, s *runtime.Service) error {
		pool := s.PoolPGTyped("primary")

		gs := s.GetGrpcServer()
		if gs == nil {
			log.Fatal("gRPC not available in monolith mode")
		}
		authpb.RegisterAuthServiceServer(gs.Server(), server.NewAuthGRPCServer(pool))

		userTbl, err := db.NewTable[models.User](pool, "users")
		if err != nil {
			return err
		}
		if err := userTbl.AutoInit(ctx); err != nil {
			return err
		}

		h, _ := auth.HashPassword("demo")
		if _, err := pool.Exec(ctx, `INSERT INTO users (username, password, role, credits) VALUES ($1,$2,$3,$4) ON CONFLICT (username) DO NOTHING`,
			"demo", h, "admin", 100); err != nil {
			return err
		}
		return nil
	})

	svc.WithRest("signup", handler.Signup(svcCtx))
	svc.WithRest("login", handler.Login(svcCtx))
	svc.WithRest("buyCredits", handler.BuyCredits(svcCtx))
	svc.WithRest("getBalance", handler.GetBalance(svcCtx))
	svc.WithRest("deductCredits", handler.DeductCredits(svcCtx))

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
