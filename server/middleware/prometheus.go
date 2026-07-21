package middleware

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/metric"
)

var (
	httpDuration = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "http_server",
		Name:      "request_duration_ms",
		Help:      "HTTP request duration in milliseconds",
		Labels:    []string{"method", "path"},
		Buckets:   []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000},
	})

	httpErrorsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "http_server",
		Name:      "errors_total",
		Help:      "HTTP error count by code",
		Labels:    []string{"method", "path", "code"},
	})
)

type metricEntry struct {
	sync.Mutex
	count  map[string]uint64
	active int64
}

var metrics = &metricEntry{
	count: make(map[string]uint64),
}

func metricKey(method, path, code string) string {
	return method + ":" + path + ":" + code
}

func Prometheus() fiber.Handler {
	return func(c fiber.Ctx) error {
		metrics.Lock()
		metrics.active++
		metrics.Unlock()
		start := time.Now()
		err := c.Next()
		dur := time.Since(start).Milliseconds()
		method := c.Method()
		path := c.Route().Path
		code := strconv.Itoa(c.Response().StatusCode())

		httpDuration.Observe(dur, method, path)
		if code >= "400" {
			httpErrorsTotal.Inc(method, path, code)
		}

		metrics.Lock()
		metrics.count[metricKey(method, path, code)]++
		metrics.active--
		metrics.Unlock()
		return err
	}
}

func PrometheusHandler() fiber.Handler {
	return func(c fiber.Ctx) error {
		metrics.Lock()
		defer metrics.Unlock()

		var b strings.Builder
		b.WriteString("# HELP http_server_requests_total Total HTTP requests\n")
		b.WriteString("# TYPE http_server_requests_total counter\n")
		for key, val := range metrics.count {
			parts := strings.SplitN(key, ":", 3)
			fmt.Fprintf(&b, "http_server_requests_total{method=%q,path=%q,code=%q} %d\n", parts[0], parts[1], parts[2], val)
		}
		b.WriteString("\n")
		b.WriteString("# HELP http_server_requests_active Active requests\n")
		b.WriteString("# TYPE http_server_requests_active gauge\n")
		fmt.Fprintf(&b, "http_server_requests_active %d\n", metrics.active)

		c.Set("Content-Type", "text/plain; version=0.0.4")
		return c.SendString(b.String())
	}
}
