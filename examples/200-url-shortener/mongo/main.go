package main

import (
	"log"
	"os"

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

	runtime.MongoMustRegister(svc, "Link", "mongo-main", "shorturl", "links", "_id")
	runtime.MongoMustRegister(svc, "LinkExpand", "mongo-main", "shorturl", "links", "shortCode")

	log.Fatal(svc.Run())
}
