package main

import (
	_ "embed"
	"log"
	"os"

	"github.com/natuleadan/sdk-api/runtime"
)

//go:embed service.docker.yaml
var dockerConfig []byte

func main() {
	cfg := dockerConfig
	if len(cfg) == 0 || os.Getenv("DOCKER_TEST") != "1" {
		config := `name: healthz
port: 23100

server:
  host: "0.0.0.0"
  prefork: true
  timeout: 30s
  body_limit: 4194304
  max_conns: 10000
  health_path: /healthz
  api_prefix: ""
  shutdown_timeout: 10s
  middleware:
    - path: "/*"
      apply:
        - logger
`
		if os.Getenv("MINIMAL") == "1" {
			config = `name: healthz
port: 23100

server:
  host: "0.0.0.0"
  prefork: true
  timeout: 30s
  body_limit: 4194304
  max_conns: 10000
  health_path: /healthz
  api_prefix: ""
  shutdown_timeout: 10s
  middleware:
    - path: "/*"
      apply: []
`
		}
		cfg = []byte(config)
	}

	svc, err := runtime.NewFromYAML(cfg)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svc.WithRest("ping", func(c *runtime.RestCtx) error {
		return c.SendString("pong")
	})

	log.Fatal(svc.Run())
}
