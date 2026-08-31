package main

import (
	"log"

	"healthz/internal/handler"
	"github.com/natuleadan/sdk-api/runtime"
)

func main() {
	config := `name: healthz
port: 23100

server:
  host: "0.0.0.0"
  prefork: true
  timeout: 30s
  body_limit: 4194304
  max_conns: 10000
  health_path: /healthz
  startup_enabled: true
  readiness_enabled: true
  liveness_enabled: true
  api_prefix: /api
  shutdown_timeout: 10s
  middleware:
    - path: "/*"
      apply:
        - logger

entry:
  - type: rest
    method: GET
    path: /ping
    handler: ping
`

	svc, err := runtime.NewFromYAML([]byte(config))
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	handler.RegisterRoutes(svc)

	log.Fatal(svc.Run())
}
