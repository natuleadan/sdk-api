package main

import (
	"log"
	"os"
	"time"

	"201-url-shortener-L1/models"

	"github.com/natuleadan/sdk-api/infra/stores/redis"
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

	runtime.MustRegister(svc, "Link", "pg-main", "link", &LinkHooks{})

	addr := os.Getenv("DRAGONFLY_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	runtime.CachedCRUD[models.LinkExpand](svc, "LinkExpand", "pg-main", "link",
		redis.RedisConf{Host: addr, Type: redis.NodeType},
		"sc:", 5*time.Minute, 30*time.Second,
	)

	log.Fatal(svc.Run())
}
