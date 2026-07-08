package main

import (
	"log"
	"os"

	"201-url-shortener/models"

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
	runtime.MustRegister(svc, "LinkExpand", "pg-main", "link", runtime.DefaultHooks[models.LinkExpand]{})

	log.Fatal(svc.Run())
}
