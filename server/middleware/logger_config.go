package middleware

import (
	"crypto/rand"
	"math/big"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type LoggerConfig struct {
	SkipPaths  []string
	SampleRate float64 // 0=log all, 0.5=log 50%, 1=log none
}

func LoggerWithConfig(cfg LoggerConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		path := string(c.Request().URI().Path())
		for _, skip := range cfg.SkipPaths {
			if path == skip || strings.HasPrefix(path, skip+"/") {
				return c.Next()
			}
		}
		if cfg.SampleRate > 0 {
			n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
			if float64(n.Int64())/1000000 < cfg.SampleRate {
				return c.Next()
			}
		}
		if traceID, ok := c.Locals("trace_id").(string); ok && traceID != "" {
			logx.Infof("[%s] %s %s %d", traceID, c.Method(), path, c.Response().StatusCode())
		} else {
			logx.Infof("%s %s %d", c.Method(), path, c.Response().StatusCode())
		}
		return c.Next()
	}
}
