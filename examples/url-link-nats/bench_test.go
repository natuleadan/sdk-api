package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	httpPort    = "18084"
	dbURL       = "postgres://dev:devpass@localhost:15432/postgres?sslmode=disable"
	natsURL     = "nats://localhost:14222"
	baseURL     = "http://localhost:" + httpPort
	hotKeys     = 100
	concurrency = 1000
	duration    = 30 * time.Second
)

var (
	setupOnce   sync.Once
	svcCmd      *exec.Cmd
	hotCodes    []string
)

func TestMain(m *testing.M) {
	_, err := buildService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	teardown()
	os.Exit(code)
}

func buildService() (string, error) {
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/url-link-nats-svc", ".").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build: %w\n%s", err, out)
	}
	return "/tmp/url-link-nats-svc", nil
}

func setup(b *testing.B) {
	setupOnce.Do(func() {
		b.Log("=== Starting containers ===")
		run("docker", "compose", "-f", "../docker-compose.yml", "up", "-d")
		waitPostgres(b)

		b.Log("=== Starting url-link-nats ===")
		svcCmd = exec.Command("/tmp/url-link-nats-svc")
		svcCmd.Env = append(os.Environ(),
			"DATABASE_URL="+dbURL,
			"NATS_URL="+natsURL,
			"PORT="+httpPort,
		)
		svcCmd.Stderr = os.Stderr
		svcCmd.Stdout = os.Stdout
		if err := svcCmd.Start(); err != nil {
			b.Fatalf("start: %v", err)
		}
		waitHTTP(b, baseURL+"/health", 10*time.Second)
		b.Log("url-link-nats ready")

		seedHotKeys(b, hotKeys)
	})
}

func seedHotKeys(b *testing.B, n int) {
	b.Logf("=== Seeding %d hot keys ===", n)
	for i := range n {
		url := fmt.Sprintf(`{"targetUrl":"https://hot-%d.example.com"}`, i)
		resp, err := http.Post(baseURL+"/api/v1/links", "application/json",
			strings.NewReader(url))
		if err != nil {
			b.Fatalf("seed %d: %v", i, err)
		}
		if resp.StatusCode != 201 {
			b.Fatalf("seed %d status=%d", i, resp.StatusCode)
		}
		var data map[string]any
		json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		if code, ok := data["shortCode"].(string); ok {
			hotCodes = append(hotCodes, code)
		}
	}
	b.Logf("Seeded %d hot keys, e.g. shortCode=%s", n, hotCodes[0])
}

func teardown() {
	if svcCmd != nil && svcCmd.Process != nil {
		svcCmd.Process.Signal(os.Interrupt)
		svcCmd.Wait()
	}
	run("docker", "compose", "-f", "../docker-compose.yml", "down", "-t", "2")
}

func waitPostgres(b *testing.B) {
	for i := 0; i < 20; i++ {
		if run("docker", "exec", "test-pg", "pg_isready", "-U", "dev") == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	b.Fatal("pg timeout")
}

func waitHTTP(b *testing.B, url string, timeout time.Duration) {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	b.Fatalf("http %s timeout", url)
}

func run(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	return cmd.Run()
}

func BenchmarkExpand(b *testing.B) {
	setup(b)

	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxConnsPerHost:     concurrency,
		MaxIdleConnsPerHost: concurrency,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	var total atomic.Int64
	done := make(chan struct{})
	start := time.Now()

	for range concurrency {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					code := hotCodes[int(total.Load())%len(hotCodes)]
					url := baseURL + "/api/v1/links/" + code
					resp, err := client.Get(url)
					if err != nil {
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					total.Add(1)
				}
			}
		}()
	}

	time.Sleep(duration)
	close(done)
	elapsed := time.Since(start)
	count := total.Load()
	rps := float64(count) / elapsed.Seconds()

	b.ReportMetric(rps, "req/s")
	b.ReportMetric(float64(count), "total_req")
	b.ReportMetric(elapsed.Seconds(), "duration_s")

	b.Logf("\n=== Results ===\nTotal requests: %d\nDuration: %.2fs\nRPS: %.0f\n================",
		count, elapsed.Seconds(), rps)

	transport.CloseIdleConnections()
}
