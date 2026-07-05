package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/trace"
)

type TraceConfig struct {
	Name     string
	Endpoint string
	Sampler  float64
	Batcher  string
}

func Trace(cfg TraceConfig) fiber.Handler {
	batcher := cfg.Batcher
	if batcher == "" {
		batcher = "otlpgrpc"
	}
	trace.StartAgent(trace.Config{
		Name:     cfg.Name,
		Endpoint: cfg.Endpoint,
		Sampler:  cfg.Sampler,
		Batcher:  batcher,
	})

	return func(c fiber.Ctx) error {
		logx.Infof("trace: %s %s", c.Method(), c.Path())
		return c.Next()
	}
}
