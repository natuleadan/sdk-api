package main

import (
	"log"

	"github.com/natuleadan/sdk-api/runtime"
	"redirects/internal/handler"
)

func main() {
	svc, err := runtime.New("service.yaml")
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	handler.RegisterRoutes(svc)

	log.Fatal(svc.Run())
}
