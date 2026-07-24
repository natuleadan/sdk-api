package main

import (
	"context"
	"log"
	"os"

	"600-grpc/account-svc/internal/handler"
	"600-grpc/account-svc/internal/models"
	"600-grpc/account-svc/internal/server"
	accountpb "600-grpc/pb/accountpb"

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

	svcCtx := handler.NewServiceContext()

	svc.WithAuthValidator(func(ctx context.Context, a *middleware.AuthContext, roles, permissions []string) error {
		return nil
	})

	svc.WithSeed(func(ctx context.Context, s *runtime.Service) error {
		pool := s.PoolPGTyped("primary")

		svcCtx.SetService(s)

		gs := s.GetGrpcServer()
		if gs != nil {
			accountpb.RegisterAccountServiceServer(gs.Server(), server.NewAccountGRPCServer(pool))
		}

		accTbl, err := db.NewTable[models.Account](pool, "accounts")
		if err != nil {
			return err
		}
		if err := accTbl.AutoInit(ctx); err != nil {
			return err
		}

		txTbl, err := db.NewTable[models.Transaction](pool, "transactions")
		if err != nil {
			return err
		}
		if err := txTbl.AutoInit(ctx); err != nil {
			return err
		}
		return nil
	})

	svc.WithExit("onTransferInitiated", func(ctx context.Context, msg []byte) ([]byte, error) {
		return handler.OnTransferInitiated(svcCtx)(ctx, msg)
	})

	svc.WithRest("createAccount", handler.CreateAccount(svcCtx))
	svc.WithRest("getBalance", handler.GetBalance(svcCtx))

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
