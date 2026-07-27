package middleware

import (
	"context"
	"hash/fnv"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func TestPrometheusSharding_ConcurrentRequests(t *testing.T) {
	logx.Disable()
	ResetMetrics()
	app := fiber.New()
	app.Use(Prometheus())
	app.Get("/metrics", PrometheusHandler())
	app.Get("/test", func(c fiber.Ctx) error { return c.SendString("ok") })

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			for range 20 {
				req := testRequest(context.Background(), "GET", "/test", nil)
				resp, _ := app.Test(req)
				if resp.StatusCode != 200 {
					t.Errorf("expected 200, got %d", resp.StatusCode)
				}
			}
		})
	}
	wg.Wait()

	req := testRequest(context.Background(), "GET", "/metrics", nil)
	resp, _ := app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "http_server_requests_total") {
		t.Error("expected metrics in response after concurrent requests")
	}
	if !strings.Contains(string(body), "http_server_requests_active 0") {
		t.Errorf("expected active=0 after all requests completed, got: %s", string(body))
	}
}

func TestPrometheusSharding_ActiveCounterAtomic(t *testing.T) {
	logx.Disable()
	ResetMetrics()
	app := fiber.New()
	app.Use(Prometheus())
	app.Get("/metrics", PrometheusHandler())
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	for range 5 {
		req := testRequest(context.Background(), "GET", "/test", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	}

	req := testRequest(context.Background(), "GET", "/metrics", nil)
	resp, _ := app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "http_server_requests_active 0") {
		t.Errorf("expected active=0 after all requests completed, got: %s", string(body))
	}
}

func TestPrometheusSharding_ShardDistribution(t *testing.T) {
	keys := []string{
		"GET:/api/v1/users:200",
		"POST:/api/v1/users:201",
		"GET:/api/v1/products:200",
		"PUT:/api/v1/users/1:200",
		"DELETE:/api/v1/users/1:204",
		"GET:/api/v1/orders:200",
		"POST:/api/v1/orders:201",
		"GET:/api/v1/health:200",
		"GET:/api/v1/metrics:200",
		"GET:/api/v1/config:200",
	}
	shardCount := make(map[int]int)
	for _, key := range keys {
		h := fnv.New32a()
		h.Write([]byte(key))
		idx := int(h.Sum32()) % numMetricShards
		shardCount[idx]++
	}

	if len(shardCount) < 3 {
		t.Errorf("expected keys to distribute across at least 3 shards, got %d: %v", len(shardCount), shardCount)
	}
}

func TestPrometheusSharding_ResetClears(t *testing.T) {
	logx.Disable()
	ResetMetrics()

	app := fiber.New()
	app.Use(Prometheus())
	app.Get("/metrics", PrometheusHandler())
	app.Get("/test", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := testRequest(context.Background(), "GET", "/test", nil)
	app.Test(req)

	ResetMetrics()

	req2 := testRequest(context.Background(), "GET", "/metrics", nil)
	resp, _ := app.Test(req2)
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), `path="/test"`) {
		t.Errorf("expected no metrics after reset, got: %s", string(body))
	}
	if !strings.Contains(string(body), "http_server_requests_active") {
		t.Error("expected active metric gauge even after reset")
	}
}

func TestMetricShardIndex_Deterministic(t *testing.T) {
	key := "GET:/test:200"
	idx1 := metricShardIndex(key)
	idx2 := metricShardIndex(key)
	if idx1 != idx2 {
		t.Errorf("metricShardIndex must be deterministic: %d != %d", idx1, idx2)
	}
	if idx1 < 0 || idx1 >= numMetricShards {
		t.Errorf("metricShardIndex out of range: %d", idx1)
	}
}

func extractMetricValue(body, metricLine string) int {
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(line, metricLine) {
			parts := strings.Split(line, " ")
			if len(parts) == 2 {
				val := 0
				for _, c := range parts[1] {
					if c >= '0' && c <= '9' {
						val = val*10 + int(c-'0')
					}
				}
				return val
			}
		}
	}
	return -1
}
