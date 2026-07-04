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
	httpPort    = 13000
	baseURL     = "http://localhost:13000"
	apiPosts    = baseURL + "/api/v1/posts"
	concurrency = 100
	benchDur    = 15 * time.Second
)

var (
	setupOnce sync.Once
	svcCmd    *exec.Cmd
	hotIDs    []int64
	hotIDMu   sync.RWMutex
	docker    bool
)

func TestMain(m *testing.M) {
	docker = os.Getenv("DOCKER_TEST") == "1"

	if !docker {
		if _, err := exec.LookPath("docker"); err != nil {
			fmt.Println("docker not found, skipping integration tests")
			os.Exit(0)
		}
		if _, err := buildService(); err != nil {
			fmt.Fprintf(os.Stderr, "build: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("=== auth-none-monolith ===")
	code := m.Run()

	if !docker {
		teardown()
	}
	os.Exit(code)
}

func buildService() (string, error) {
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/auth-none-monolith-svc", ".").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build: %w\n%s", err, out)
	}
	return "/tmp/auth-none-monolith-svc", nil
}

func setup(tb testing.TB) {
	if docker {
		setupDocker(tb)
		return
	}
	setupOnce.Do(func() {
		tb.Log("starting postgres...")
		run(tb, "docker", "compose", "down", "-t", "2", "-v")
		run(tb, "docker", "compose", "up", "-d", "postgres")
		waitPostgres(tb, "none-mono-pg")

		tb.Log("starting service...")
		svcCmd = exec.Command("/tmp/auth-none-monolith-svc")
		svcCmd.Env = append(os.Environ(), "DATABASE_URL=postgres://postgres:postgres@localhost:15432/postgres?sslmode=disable")
		svcCmd.Stderr = os.Stderr
		if err := svcCmd.Start(); err != nil {
			tb.Fatalf("start: %v", err)
		}
		waitHTTP(tb, baseURL+"/health", 15*time.Second)
		tb.Log("service ready")

		seedPosts(tb, 20)
	})
}

func setupDocker(tb testing.TB) {
	setupOnce.Do(func() {
		waitHTTP(tb, baseURL+"/health", 30*time.Second)
		tb.Log("service ready (Docker)")
		seedPosts(tb, 20)
	})
}

func seedPosts(tb testing.TB, n int) {
	tb.Logf("seeding %d posts...", n)
	for i := range n {
		body := fmt.Sprintf(`{"title":"Post %d","content":"Content %d"}`, i, i)
		resp, err := http.Post(apiPosts, "application/json", strings.NewReader(body))
		if err != nil {
			tb.Fatalf("seed post %d: %v", i, err)
		}
		if resp.StatusCode != 201 {
			b, _ := io.ReadAll(resp.Body)
			tb.Fatalf("seed post %d status=%d body=%s", i, resp.StatusCode, string(b))
		}
		var created map[string]any
		json.NewDecoder(resp.Body).Decode(&created)
		resp.Body.Close()
		if id, ok := created["id"].(float64); ok {
			hotIDMu.Lock()
			hotIDs = append(hotIDs, int64(id))
			hotIDMu.Unlock()
		}
	}
	tb.Logf("seeded %d posts", n)
}

func teardown() {
	if svcCmd != nil && svcCmd.Process != nil {
		svcCmd.Process.Signal(os.Interrupt)
		svcCmd.Wait()
	}
	run(nil, "docker", "compose", "down", "-t", "2", "-v")
}

func waitPostgres(tb testing.TB, container string) {
	for i := range 20 {
		if run(nil, "docker", "exec", container, "pg_isready", "-U", "postgres") == nil {
			return
		}
		tb.Logf("waiting for postgres... (%d/20)", i+1)
		time.Sleep(500 * time.Millisecond)
	}
	tb.Fatal("postgres timeout")
}

func waitHTTP(tb testing.TB, url string, timeout time.Duration) {
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
	tb.Fatalf("service not ready after %v", timeout)
}

func run(tb testing.TB, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	if tb != nil {
		cmd.Stderr = os.Stderr
	}
	err := cmd.Run()
	if err != nil && tb != nil {
		tb.Logf("run: %v: %v", args, err)
	}
	return err
}

func httpGet(url string) (*http.Response, error) {
	return http.Get(url)
}

func httpPost(url, body string) (*http.Response, error) {
	return http.Post(url, "application/json", strings.NewReader(body))
}

func httpDelete(url string) (*http.Response, error) {
	req, _ := http.NewRequest("DELETE", url, nil)
	return http.DefaultClient.Do(req)
}

// --- Functional tests ---

func TestNone_CRUD_CreateAndList(t *testing.T) {
	setup(t)

	t.Run("create post returns 201", func(t *testing.T) {
		body := `{"title":"test-create","content":"hello world"}`
		resp, err := httpPost(apiPosts, body)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("list posts returns 200 with array", func(t *testing.T) {
		resp, err := httpGet(apiPosts)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		if items, ok := result["data"].([]any); !ok || len(items) < 1 {
			t.Error("expected at least 1 post in data array")
		}
	})
}

func TestNone_NoAuthHeader_Works(t *testing.T) {
	setup(t)

	body := `{"title":"no-auth","content":"no auth header"}`
	resp, err := httpPost(apiPosts, body)
	if err != nil {
		t.Fatalf("no-auth create: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Errorf("expected 201 without auth header, got %d", resp.StatusCode)
	}
}

func TestNone_AuthDisabled_Proof(t *testing.T) {
	setup(t)

	t.Run("GET without any auth header works", func(t *testing.T) {
		resp, err := httpGet(apiPosts)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("expected 200 without auth, got %d", resp.StatusCode)
		}
	})

	t.Run("POST with fake JWT still works (auth disabled)", func(t *testing.T) {
		body := `{"title":"fake-jwt","content":"with fake token"}`
		req, _ := http.NewRequest("POST", apiPosts, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwicm9sZXMiOlsiYWRtaW4iXSwiZXhwIjo5OTk5OTk5OTk5fQ.fake-signature")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post with fake JWT: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Errorf("auth disabled: expected 201 with any JWT, got %d", resp.StatusCode)
		}
	})

	t.Run("POST with FGA-style auth headers still works", func(t *testing.T) {
		body := `{"title":"fga-headers","content":"with fga-like headers"}`
		req, _ := http.NewRequest("POST", apiPosts, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-OpenFGA-Store-ID", "fake-store")
		req.Header.Set("X-Zitadel-Org-ID", "fake-org")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post with FGA headers: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Errorf("auth disabled: expected 201 with FGA headers, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE without auth works", func(t *testing.T) {
		hotIDMu.RLock()
		if len(hotIDs) == 0 {
			hotIDMu.RUnlock()
			t.Skip("no seeded ids to delete")
			return
		}
		id := hotIDs[0]
		hotIDMu.RUnlock()

		resp, err := httpDelete(fmt.Sprintf("%s/%d", apiPosts, id))
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 204 {
			t.Errorf("expected 204 without auth, got %d", resp.StatusCode)
		}
	})
}

func TestNone_ConcurrentRequests(t *testing.T) {
	setup(t)

	var ok, fail atomic.Int64
	var wg sync.WaitGroup
	workers := 50
	iters := 20

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iters {
				resp, err := httpGet(apiPosts)
				if err != nil || resp.StatusCode != 200 {
					fail.Add(1)
					if resp != nil {
						resp.Body.Close()
					}
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				ok.Add(1)
			}
		}()
	}
	wg.Wait()

	if fail.Load() > 0 {
		t.Errorf("concurrent requests: %d failures out of %d", fail.Load(), ok.Load()+fail.Load())
	}
	t.Logf("concurrent: %d ok, %d fail", ok.Load(), fail.Load())
}

// --- Benchmarks ---

func BenchmarkNone_GetRPS(b *testing.B) {
	setup(b)

	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxConnsPerHost:     concurrency,
		MaxIdleConnsPerHost: concurrency,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	var total atomic.Int64
	var errs atomic.Int64
	done := make(chan struct{})
	start := time.Now()

	for range concurrency {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					resp, err := client.Get(apiPosts)
					if err != nil {
						errs.Add(1)
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					total.Add(1)
				}
			}
		}()
	}

	time.Sleep(benchDur)
	close(done)
	elapsed := time.Since(start)
	count := total.Load()
	rps := float64(count) / elapsed.Seconds()

	b.ReportMetric(rps, "req/s")
	b.ReportMetric(float64(errs.Load()), "errors")
	b.Logf("\n=== Benchmark None GET ===\nDocker: %v\nTotal: %d\nDuration: %.2fs\nRPS: %.0f\nErrors: %d\n=========================",
		docker, count, elapsed.Seconds(), rps, errs.Load())
	transport.CloseIdleConnections()
}

func BenchmarkNone_CreateRPS(b *testing.B) {
	setup(b)

	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxConnsPerHost:     concurrency,
		MaxIdleConnsPerHost: concurrency,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	var total atomic.Int64
	var errs atomic.Int64
	done := make(chan struct{})
	start := time.Now()

	for range concurrency {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					body := fmt.Sprintf(`{"title":"bench-%d","content":"benchmark"}`, total.Load())
					resp, err := client.Post(apiPosts, "application/json", strings.NewReader(body))
					if err != nil {
						errs.Add(1)
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode == 201 {
						total.Add(1)
					} else {
						errs.Add(1)
					}
				}
			}
		}()
	}

	time.Sleep(benchDur)
	close(done)
	elapsed := time.Since(start)
	count := total.Load()
	rps := float64(count) / elapsed.Seconds()

	b.ReportMetric(rps, "req/s")
	b.ReportMetric(float64(errs.Load()), "errors")
	b.Logf("\n=== Benchmark None CREATE ===\nDocker: %v\nTotal: %d\nDuration: %.2fs\nRPS: %.0f\nErrors: %d\n=========================",
		docker, count, elapsed.Seconds(), rps, errs.Load())
	transport.CloseIdleConnections()
}
