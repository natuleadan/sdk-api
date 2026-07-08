package main

import (
	"log"
	"os"

	"201-url-shortener-turso/models"

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

	runtime.TursoMustRegister[models.Link](svc, "Link", "turso-main", "link", &LinkHooks{})
	runtime.TursoMustRegister[models.LinkExpand](svc, "LinkExpand", "turso-main", "link", nil)

	log.Fatal(svc.Run())
}
