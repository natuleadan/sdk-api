package middleware

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/metric"
)

const numMetricShards = 16

var httpDuration = metric.NewHistogramVec(&metric.HistogramVecOpts{
	Namespace: "http_server",
	Name:      "request_duration_ms",
	Help:      "HTTP request duration in milliseconds",
	Labels:    []string{"method", "path"},
	Buckets:   []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000},
})

type metricShard struct {
	mu    sync.Mutex
	count map[string]uint64
}

type shardedMetrics struct {
	shards [numMetricShards]metricShard
	active atomic.Int64
}

func newShardedMetrics() *shardedMetrics {
	sm := &shardedMetrics{}
	for i := range numMetricShards {
		sm.shards[i].count = make(map[string]uint64)
	}
	return sm
}

var metrics = newShardedMetrics()

func metricKey(method, path, code string) string {
	return method + ":" + path + ":" + code
}

func metricShardIndex(key string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32()) % numMetricShards
}

func Prometheus() fiber.Handler {
	return func(c fiber.Ctx) error {
		reqPath := string(c.Request().URI().Path())
		if reqPath == "/metrics" || reqPath == "/health" {
			return c.Next()
		}
		routePath := c.Route().Path
		if routePath == "" {
			routePath = reqPath
		}
		metrics.active.Add(1)
		start := time.Now()
		err := c.Next()
		dur := time.Since(start).Milliseconds()
		method := c.Method()
		code := strconv.Itoa(c.Response().StatusCode())

		httpDuration.Observe(dur, method, routePath)

		key := metricKey(method, routePath, code)
		shard := &metrics.shards[metricShardIndex(key)]
		shard.mu.Lock()
		shard.count[key]++
		shard.mu.Unlock()
		metrics.active.Add(-1)
		return err
	}
}

func PrometheusHandler() fiber.Handler {
	return func(c fiber.Ctx) error {
		var total uint64
		var b strings.Builder
		b.WriteString("# HELP http_server_requests_total Total HTTP requests\n")
		b.WriteString("# TYPE http_server_requests_total counter\n")

		var shardMaps [numMetricShards]map[string]uint64
		for i := range numMetricShards {
			shard := &metrics.shards[i]
			shard.mu.Lock()
			shardMaps[i] = shard.count
			shard.mu.Unlock()
		}

		for _, snap := range shardMaps {
			for key, val := range snap {
				parts := strings.SplitN(key, ":", 3)
				fmt.Fprintf(&b, "http_server_requests_total{method=%q,path=%q,code=%q} %d\n", parts[0], parts[1], parts[2], val)
				total += val
			}
		}
		b.WriteString("\n")
		b.WriteString("# HELP http_server_requests_active Active requests\n")
		b.WriteString("# TYPE http_server_requests_active gauge\n")
		fmt.Fprintf(&b, "http_server_requests_active %d\n", metrics.active.Load())

		c.Set("Content-Type", "text/plain; version=0.0.4")
		return c.SendString(b.String())
	}
}

// ResetMetrics clears all collected metrics. Used in tests.
func ResetMetrics() {
	m := newShardedMetrics()
	for i := range numMetricShards {
		shard := &metrics.shards[i]
		shard.mu.Lock()
		shard.count = make(map[string]uint64)
		shard.mu.Unlock()
	}
	metrics.active.Store(0)
	_ = m
}
