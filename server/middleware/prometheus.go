package middleware

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type metricEntry struct {
	sync.Mutex
	count    map[string]uint64
	duration map[string]float64
	active   int64
}

var metrics = &metricEntry{
	count:    make(map[string]uint64),
	duration: make(map[string]float64),
}

func metricKey(method, path, code string) string {
	return method + ":" + path + ":" + code
}

func durationKey(method, path string) string {
	return method + ":" + path
}

func Prometheus() fiber.Handler {
	return func(c *fiber.Ctx) error {
		metrics.Lock()
		metrics.active++
		metrics.Unlock()
		start := time.Now()
		err := c.Next()
		dur := float64(time.Since(start).Microseconds()) / 1000
		method := c.Method()
		path := c.Route().Path
		code := strconv.Itoa(c.Response().StatusCode())

		metrics.Lock()
		metrics.count[metricKey(method, path, code)]++
		metrics.duration[durationKey(method, path)] += dur
		metrics.active--
		metrics.Unlock()
		return err
	}
}

func PrometheusHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
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
		b.WriteString("# HELP http_server_request_duration_ms HTTP request duration in ms\n")
		b.WriteString("# TYPE http_server_request_duration_ms summary\n")
		for key, val := range metrics.duration {
			parts := strings.SplitN(key, ":", 2)
		fmt.Fprintf(&b, "http_server_request_duration_ms{method=%q,path=%q,quantile=\"0.99\"} %.2f\n", parts[0], parts[1], val)
		}
		b.WriteString("\n")
		b.WriteString("# HELP http_server_requests_active Active requests\n")
		b.WriteString("# TYPE http_server_requests_active gauge\n")
		fmt.Fprintf(&b, "http_server_requests_active %d\n", metrics.active)

		c.Set("Content-Type", "text/plain; version=0.0.4")
		return c.SendString(b.String())
	}
}
