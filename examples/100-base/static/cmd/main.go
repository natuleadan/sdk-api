package main

import (
	"log"

	"static/internal/handler"
	"github.com/natuleadan/sdk-api/runtime"
)

func main() {
	svc, err := runtime.New("service.yaml")
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	handler.RegisterRoutes(svc)

	log.Fatal(svc.Run())
}
