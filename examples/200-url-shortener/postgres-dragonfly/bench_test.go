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
	baseURL     = "http://localhost:23204"
	apiLinks    = baseURL + "/api/v1/links"
	apiExpand   = baseURL + "/api/v1/expand"
	benchDur    = 15 * time.Second
	concurrency = 100
)

var (
	setupOnce sync.Once
	svcCmd    *exec.Cmd
	hotCodes  []string
	hotMu     sync.RWMutex
	docker    bool
)

func TestMain(m *testing.M) {
	docker = os.Getenv("DOCKER_TEST") == "1"

	if !docker {
		if _, err := exec.LookPath("docker"); err != nil {
			fmt.Println("skip: no docker")
			os.Exit(0)
		}
		out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/url-svc", ".").CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
			os.Exit(1)
		}
	}

	fmt.Println("=== url-shortener ===")
	code := m.Run()

	if !docker {
		teardown()
	}
	os.Exit(code)
}

func setup(tb testing.TB) {
	if docker {
		setupOnce.Do(func() { waitHTTP(tb, baseURL+"/health", 30*time.Second) })
		return
	}
	setupOnce.Do(func() {
		tbLog(tb, "starting containers...")
		run("docker", "compose", "down", "-t", "2", "-v")
		run("docker", "compose", "up", "-d", "postgres", "pgdog")
		waitPostgres(tb)

		tbLog(tb, "starting service...")
		svcCmd = exec.Command("/tmp/url-svc")
		svcCmd.Env = append(os.Environ(),
			"DATABASE_URL=postgres://dev:devpass@localhost:24204/postgres?sslmode=disable",
		)
		svcCmd.Stderr = os.Stderr
		if err := svcCmd.Start(); err != nil {
			tb.Fatalf("start: %v", err)
		}
		waitHTTP(tb, baseURL+"/health", 15*time.Second)
		tbLog(tb, "service ready")

		if len(hotCodes) == 0 {
			seedKeys(tb, 100)
		}
	})
}

func tbLog(tb testing.TB, msg string) {
	if h, ok := tb.(interface{ Helper() }); ok {
		h.Helper()
	}
	tb.Log(msg)
}

func seedKeys(tb testing.TB, n int) {
	for i := range n {
		code := fmt.Sprintf("hot%05d", i)
		body := fmt.Sprintf(`{"targetUrl":"https://hot-%d.example.com","shortCode":"%s"}`, i, code)
		resp, err := http.Post(apiLinks, "application/json", strings.NewReader(body))
		if err != nil {
			tb.Fatalf("seed %d: %v", i, err)
		}
		if resp.StatusCode != 201 {
			tb.Fatalf("seed %d status=%d", i, resp.StatusCode)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		hotMu.Lock()
		hotCodes = append(hotCodes, code)
		hotMu.Unlock()
	}
	tb.Logf("seeded %d keys, e.g. %s", n, hotCodes[0])
}

func teardown() {
	if svcCmd != nil && svcCmd.Process != nil {
		svcCmd.Process.Signal(os.Interrupt)
		svcCmd.Wait()
	}
	run("docker", "compose", "down", "-t", "2", "-v")
}

func waitPostgres(tb testing.TB) {
	for i := range 20 {
		if run("docker", "compose", "exec", "-T", "postgres", "pg_isready", "-U", "dev") == nil {
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

func run(args ...string) error {
	return exec.Command(args[0], args[1:]...).Run()
}

func TestURL_CreateAndExpand(t *testing.T) {
	setup(t)

	body := `{"targetUrl":"https://example.com/test-create"}`
	resp, err := http.Post(apiLinks, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Errorf("create: expected 201, got %d", resp.StatusCode)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	code, _ := created["shortCode"].(string)
	if code == "" {
		t.Fatal("no shortCode returned")
	}
	t.Logf("created shortCode=%s", code)

	expResp, err := http.Get(apiExpand + "/" + code)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	defer expResp.Body.Close()
	if expResp.StatusCode != 200 {
		t.Errorf("expand: expected 200, got %d", expResp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(expResp.Body).Decode(&result)
	if url, _ := result["targetUrl"].(string); url != "https://example.com/test-create" {
		t.Errorf("expand: expected targetUrl, got %v", result)
	}
}

func TestURL_CRUDLifecycle(t *testing.T) {
	setup(t)

	body := `{"targetUrl":"https://lifecycle.example.com"}`
	resp, err := http.Post(apiLinks, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Errorf("create: expected 201, got %d", resp.StatusCode)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	id, ok := created["id"].(float64)
	if !ok {
		t.Fatal("no id returned")
	}
	t.Logf("created id=%.0f", id)

	getResp, err := http.Get(fmt.Sprintf("%s/%.0f", apiLinks, id))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != 200 {
		t.Errorf("get: expected 200, got %d", getResp.StatusCode)
	}

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/%.0f", apiLinks, id), nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Errorf("delete: expected 204, got %d", delResp.StatusCode)
	}
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
					hotMu.RLock()
					code := hotCodes[int(total.Load())%len(hotCodes)]
					hotMu.RUnlock()
					resp, err := client.Get(apiExpand + "/" + code)
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
	b.Logf("\n=== Results ===\nTotal: %d\nDuration: %.2fs\nRPS: %.0f\nErrors: %d\n================",
		count, elapsed.Seconds(), rps, errs.Load())

	transport.CloseIdleConnections()
}
