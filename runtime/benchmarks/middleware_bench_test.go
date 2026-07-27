package benchmarks

import (
	"context"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func BenchmarkMiddlewareChain(b *testing.B) {
	logx.Disable()
	runs := []struct {
		name  string
		setup func() *fiber.App
	}{
		{
			name: "no_middleware",
			setup: func() *fiber.App {
				app := fiber.New()
				app.Get("/", func(c fiber.Ctx) error {
					return c.SendString("ok")
				})
				return app
			},
		},
		{
			name: "prometheus_only",
			setup: func() *fiber.App {
				app := fiber.New()
				app.Use(middleware.Prometheus())
				app.Get("/", func(c fiber.Ctx) error {
					return c.SendString("ok")
				})
				return app
			},
		},
		{
			name: "bodyreader_prometheus",
			setup: func() *fiber.App {
				app := fiber.New()
				app.Use(middleware.BodyReader())
				app.Use(middleware.Prometheus())
				app.Get("/", func(c fiber.Ctx) error {
					return c.SendString("ok")
				})
				return app
			},
		},
		{
			name: "bodyreader_maxbytes_prometheus",
			setup: func() *fiber.App {
				app := fiber.New()
				app.Use(middleware.BodyReader())
				app.Use(middleware.MaxBytes(1 << 20))
				app.Use(middleware.Prometheus())
				app.Get("/", func(c fiber.Ctx) error {
					return c.SendString("ok")
				})
				return app
			},
		},
	}

	for _, r := range runs {
		b.Run(r.name, func(b *testing.B) {
			app := r.setup()
			req, _ := http.NewRequestWithContext(context.Background(), "GET", "/", nil)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				resp, _ := app.Test(req)
				if resp.StatusCode != 200 {
					b.Fatalf("expected 200, got %d", resp.StatusCode)
				}
			}
		})
	}
}

func BenchmarkPrometheusSequential(b *testing.B) {
	logx.Disable()
	middleware.ResetMetrics()
	app := fiber.New()
	app.Use(middleware.Prometheus())
	app.Get("/bench", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/bench", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			b.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	}
}
