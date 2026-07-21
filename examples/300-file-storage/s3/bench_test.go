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
	baseURL     = "http://localhost:23305"
	apiUpload   = baseURL + "/api/files/upload"
	apiDownload = baseURL + "/api/files/download"
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
		out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/file-s3-svc", ".").CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
			os.Exit(1)
		}
	}

	fmt.Println("=== file-s3 ===")
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
		seedURL := os.Getenv("DATABASE_URL")
		if seedURL == "" {
			seedURL = "postgres://dev:devpass@localhost:15435/postgres?sslmode=disable"
		}

		svcCmd = exec.Command("/tmp/file-s3-svc")
		svcCmd.Env = append(os.Environ(), "PORT=23305")
		svcCmd.Stdout = os.Stdout
		svcCmd.Stderr = os.Stderr
		if err := svcCmd.Start(); err != nil {
			tb.Fatalf("start svc: %v", err)
		}
		waitHTTP(tb, baseURL+"/health", 30*time.Second)
	})
}

func teardown() {
	if svcCmd != nil && svcCmd.Process != nil {
		svcCmd.Process.Kill()
		svcCmd.Wait()
	}
}

func waitHTTP(tb testing.TB, url string, timeout time.Duration) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	tb.Fatalf("timeout waiting for %s", url)
}

func seedKey(tb testing.TB, code string) {
	tb.Helper()
	url := apiUpload + "/" + code + ".dat"
	body := strings.NewReader("seed-data-" + code)
	resp, err := http.Post(url, "application/octet-stream", body)
	if err != nil {
		tb.Fatalf("seed %s: %v", code, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

func TestFile_UploadAndDownload(t *testing.T) {
	setup(t)
	code := fmt.Sprintf("test-%d", time.Now().UnixNano())
	uploadURL := apiUpload + "/" + code + ".dat"
	payload := "hello-file-s3-" + code

	resp, err := http.Post(uploadURL, "text/plain", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status %d: %s", resp.StatusCode, string(body))
	}

	downloadURL := apiDownload + "/" + code + ".dat"
	resp2, err := http.Get(downloadURL)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("download status %d: %s", resp2.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp2.Body)
	if string(body) != payload {
		t.Fatalf("download body mismatch: got %q, want %q", string(body), payload)
	}
}

func TestFile_ContentIntegrity(t *testing.T) {
	setup(t)
	code := fmt.Sprintf("integrity-%d", time.Now().UnixNano())
	payload := strings.Repeat("Hello World-Check-", 1000)

	resp, err := http.Post(apiUpload+"/"+code+".dat", "text/plain", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status %d: %s", resp.StatusCode, string(body))
	}

	resp2, err := http.Get(apiDownload + "/" + code + ".dat")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("download status %d: %s", resp2.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp2.Body)
	if string(body) != payload {
		t.Fatalf("content mismatch: len got=%d want=%d", len(body), len(payload))
	}
}

func TestFile_UploadTwice(t *testing.T) {
	setup(t)
	code := fmt.Sprintf("twice-%d", time.Now().UnixNano())
	url := apiUpload + "/" + code + ".dat"

	resp, _ := http.Post(url, "text/plain", strings.NewReader("first"))
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("first upload status %d", resp.StatusCode)
	}

	resp2, _ := http.Post(url, "text/plain", strings.NewReader("second"))
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("second upload status %d", resp2.StatusCode)
	}

	resp3, _ := http.Get(apiDownload + "/" + code + ".dat")
	defer resp3.Body.Close()
	body, _ := io.ReadAll(resp3.Body)
	if string(body) != "second" {
		t.Fatalf("overwrite check: got %q, want 'second'", string(body))
	}
}

func TestFile_FileNotFound(t *testing.T) {
	setup(t)
	resp, err := http.Get(apiDownload + "/nonexistent-file.dat")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestFile_PresignRedirect(t *testing.T) {
	setup(t)
	code := fmt.Sprintf("presign-%d", time.Now().UnixNano())
	payload := "presign-test-data"

	resp, _ := http.Post(apiUpload+"/"+code+".dat", "text/plain", strings.NewReader(payload))
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("upload status %d", resp.StatusCode)
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp2, err := client.Get(baseURL + "/api/files/presign/" + code + ".dat")
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 302 {
		t.Fatalf("presign expected 302, got %d", resp2.StatusCode)
	}
	loc := resp2.Header.Get("Location")
	if loc == "" {
		t.Fatal("presign missing Location header")
	}
	if !strings.Contains(loc, "X-Amz-Signature") {
		t.Fatalf("presign URL missing signature: %s", loc)
	}
	t.Logf("presigned URL valid: %.60s...", loc)
}

func TestFile_SignOnlyJSON(t *testing.T) {
	setup(t)
	code := fmt.Sprintf("sign-%d", time.Now().UnixNano())

	// Upload first so the file exists
	_, _ = http.Post(apiUpload+"/"+code+".dat", "text/plain", strings.NewReader("sign-data"))

	resp, err := http.Get(baseURL + "/api/files/sign/" + code + ".dat")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("sign status %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	url, _ := body["url"].(string)
	if url == "" {
		t.Fatal("sign: expected url in JSON response")
	}
	if !strings.Contains(url, "X-Amz-Signature") {
		t.Fatalf("sign: URL missing signature: %s", url)
	}
	key, _ := body["key"].(string)
	if key == "" {
		t.Fatal("sign: expected key in JSON response")
	}
	exp, _ := body["expires"].(string)
	if exp == "" {
		t.Error("sign: expected expires in JSON response")
	}
	t.Logf("sign: url=%.60s... key=%s expires=%s", url, key, exp)
}

func BenchmarkUpload(b *testing.B) {
	setup(b)
	payload := strings.Repeat("a", 1024)
	payloadSize := int64(len(payload))

	transport := &http.Transport{
		MaxIdleConns:        concurrency,
		MaxConnsPerHost:     concurrency,
		MaxIdleConnsPerHost: concurrency,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	var id atomic.Int64
	var total atomic.Int64
	var errs atomic.Int64
	var bytes atomic.Int64
	done := make(chan struct{})
	start := time.Now()

	for range concurrency {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					uid := id.Add(1)
					url := apiUpload + fmt.Sprintf("/bench-%d.dat", uid)
					resp, err := client.Post(url, "application/octet-stream", strings.NewReader(payload))
					if err != nil {
						errs.Add(1)
						continue
					}
					n, _ := io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					total.Add(1)
					bytes.Add(n + payloadSize)
				}
			}
		}()
	}

	time.Sleep(benchDur)
	close(done)
	elapsed := time.Since(start)
	count := total.Load()
	rps := float64(count) / elapsed.Seconds()
	mbps := float64(bytes.Load()) / elapsed.Seconds() / 1_000_000

	b.ReportMetric(rps, "req/s")
	b.ReportMetric(mbps, "MB/s")
	b.ReportMetric(float64(errs.Load()), "errors")
	b.Logf("\n=== Results ===\nTotal: %d\nErrors: %d\nRPS: %.0f\nMB/s: %.2f\n================",
		count, errs.Load(), rps, mbps)

	transport.CloseIdleConnections()
}

func BenchmarkDownload(b *testing.B) {
	setup(b)

	code := fmt.Sprintf("bench-%d", time.Now().UnixNano())
	seedKey(b, code)
	downloadURL := apiDownload + "/" + code + ".dat"

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
	var bytes atomic.Int64
	done := make(chan struct{})
	start := time.Now()

	for range concurrency {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					resp, err := client.Get(downloadURL)
					if err != nil {
						errs.Add(1)
						continue
					}
					n, _ := io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					total.Add(1)
					bytes.Add(n)
				}
			}
		}()
	}

	time.Sleep(benchDur)
	close(done)
	elapsed := time.Since(start)
	count := total.Load()
	rps := float64(count) / elapsed.Seconds()
	mbps := float64(bytes.Load()) / elapsed.Seconds() / 1_000_000

	b.ReportMetric(rps, "req/s")
	b.ReportMetric(mbps, "MB/s")
	b.ReportMetric(float64(errs.Load()), "errors")
	b.Logf("\n=== Results ===\nTotal: %d\nErrors: %d\nRPS: %.0f\nMB/s: %.2f\n================",
		count, errs.Load(), rps, mbps)

	transport.CloseIdleConnections()
}
