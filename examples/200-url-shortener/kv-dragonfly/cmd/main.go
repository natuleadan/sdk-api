package main

import (
	"log"
	"os"

	"kv-dragonfly-v2/internal/handler"
	appsvc "kv-dragonfly-v2/internal/svc"
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

	rdb := s.KV("kv-main")
	svcCtx := appsvc.NewServiceContext(rdb)
	handler.RegisterRoutes(s, svcCtx)

	log.Fatal(s.Run())
}
