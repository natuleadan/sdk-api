package main

import (
	"log"
	"os"

	"201-url-shortener-maria/models"

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

	runtime.MySQLMustRegister[models.Link](svc, "Link", "maria-main", "link", &LinkHooks{})
	runtime.MySQLMustRegister[models.LinkExpand](svc, "LinkExpand", "maria-main", "link", nil)

	log.Fatal(svc.Run())
}
