package main

import (
	"log"
	"os"

	"url-shortener-nats/internal/handler"
	"url-shortener-nats/internal/svc"
	"url-shortener-nats/models"

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
	models.SetHooksBridge(svcCtx)
	runtime.MustRegister[models.Link](s, "Link", "pg-main", "link", &models.LinkHooks{CodeByID: make(map[int64]string)})

	handler.RegisterRoutes(s, svcCtx)

	s.WithExit("onLinkEvent", svcCtx.OnLinkEvent)
	s.WithExit("onPullMsg", svcCtx.OnPullMsg)

	go svcCtx.StartRPCCoreSub()

	if err := s.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
