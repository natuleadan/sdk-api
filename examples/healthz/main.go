package main

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/server"
)

func main() {
	cfg := server.Config{
		Port:           18081,
		Host:           "0.0.0.0",
		Prefork:        true,
		BodyLimit:      4 << 20,
		MaxBytes:       4 << 20,
		MaxConns:       10000,
		Timeout:        30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		MetricsPath:    "/metrics",
		HealthPath:     "/healthz",
		RecoverStack:   false,
	}

	if os.Getenv("MINIMAL") == "1" {
		cfg.Routes = []server.RouteConfig{
			{Path: "/", Middleware: []string{}},
		}
	}

	srv := server.New(cfg, server.TelemetryConfig{}, server.SecurityConfig{}, nil)
	srv.App().Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})
	srv.App().Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("pong")
	})
	log.Fatal(srv.Start())
}
