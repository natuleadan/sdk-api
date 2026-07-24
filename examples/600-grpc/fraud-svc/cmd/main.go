package main

import (
	"log"
	"os"

	"600-grpc/fraud-svc/internal/handler"

	"github.com/natuleadan/sdk-api/runtime"
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
	svc.WithRest("checkFraud", handler.CheckFraud(svcCtx))

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
