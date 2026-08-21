package main

import (
	"log"
	"strings"

	"scalar-ui/internal/handler"
	"github.com/natuleadan/sdk-api/runtime"
)

func main() {
	svc, err := runtime.New("service.yaml")
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	handler.RegisterRoutes(svc)

	// Dynamic origin validation for the "app" group: in addition to the YAML
	// allowlist (https://app.example.com), any *.trusted.example.com passes.
	svc.SetCORSOriginsFunc("app", func(origin string) bool {
		return strings.HasSuffix(origin, ".trusted.example.com")
	})

	log.Fatal(svc.Run())
}
