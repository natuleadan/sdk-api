package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

const (
	httpPort = "23104"
	baseURL  = "http://localhost:" + httpPort
)

var docker bool

func TestMain(m *testing.M) {
	docker = os.Getenv("DOCKER_TEST") == "1"
	if !docker {
		if _, err := exec.LookPath("go"); err != nil {
			fmt.Println("skip: no go compiler")
			os.Exit(0)
		}
	}
	if !docker {
		exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/static-svc", "./cmd/").Run()
	}
	os.Exit(m.Run())
}

func TestStatic_Assets(t *testing.T) {
	waitForService(t)
	resp := httpGet(t, "/assets/style.css")
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	body, _ := io.ReadAll(resp.Body)
	assertContains(t, string(body), "body{margin:0}")
}

func TestStatic_SPAFallback(t *testing.T) {
	waitForService(t)
	resp := httpGet(t, "/app/any/route")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assertContains(t, string(body), "<div id=app></div>")
}

func TestStatic_SPAServesExistingFiles(t *testing.T) {
	waitForService(t)
	resp := httpGet(t, "/app/app.js")
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	body, _ := io.ReadAll(resp.Body)
	assertContains(t, string(body), "console.log('spa')")
}

func TestStatic_Browse(t *testing.T) {
	waitForService(t)
	resp := httpGet(t, "/files/")
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	body, _ := io.ReadAll(resp.Body)
	assertContains(t, string(body), "readme.txt")
}

func TestStatic_Download(t *testing.T) {
	waitForService(t)
	resp := httpGet(t, "/downloads/report.pdf")
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	disp := resp.Header.Get("Content-Disposition")
	assertContains(t, disp, "attachment")
}

func TestStatic_CustomIndex(t *testing.T) {
	waitForService(t)
	resp := httpGet(t, "/docs/")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assertContains(t, string(body), "Custom Index")
}

func TestStatic_MaxAge(t *testing.T) {
	waitForService(t)
	resp := httpGet(t, "/assets/style.css")
	defer resp.Body.Close()
	cc := resp.Header.Get("Cache-Control")
	assertContains(t, cc, "max-age=3600")
}

func TestStatic_NotFound(t *testing.T) {
	waitForService(t)
	resp := httpGet(t, "/assets/missing.txt")
	defer resp.Body.Close()
	assertStatus(t, resp, 404)
}

func BenchmarkStatic(b *testing.B) {
	if !docker {
		svcPath, err := buildService()
		if err != nil {
			b.Fatalf("build: %v", err)
		}
		cmd := startService(b, svcPath)
		defer cmd.Process.Kill()
	}
	waitHTTP(b, baseURL+"/healthz", 30*time.Second)

	transport := &http.Transport{MaxIdleConns: 100, MaxConnsPerHost: 100, IdleConnTimeout: 90 * time.Second}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	b.Run("static", func(b *testing.B) {
		b.ResetTimer()
		var wg sync.WaitGroup
		workers := 10
		iterPerWorker := b.N / workers
		for range workers {
			wg.Go(func() {
				for range iterPerWorker {
					resp, err := client.Get(baseURL + "/assets/style.css")
					if err != nil {
						b.Errorf("request failed: %v", err)
						return
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			})
		}
		wg.Wait()
	})
	transport.CloseIdleConnections()
}

func buildService() (string, error) {
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/static-svc", "./cmd/").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build: %w\n%s", err, out)
	}
	return "/tmp/static-svc", nil
}

func startService(b testing.TB, path string) *exec.Cmd {
	cmd := exec.Command(path)
	cmd.Env = append(os.Environ(), "PORT="+httpPort)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		b.Fatalf("start: %v", err)
	}
	return cmd
}

func waitForService(tb testing.TB) {
	tb.Helper()
	waitHTTP(tb, baseURL+"/healthz", 30*time.Second)
}

func waitHTTP(tb testing.TB, url string, timeout time.Duration) {
	tb.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	tb.Fatalf("service not ready after %v", timeout)
}

func httpGet(tb testing.TB, path string) *http.Response {
	tb.Helper()
	resp, err := http.Get(baseURL + path)
	if err != nil {
		tb.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func assertStatus(tb testing.TB, resp *http.Response, want int) {
	tb.Helper()
	if resp.StatusCode != want {
		tb.Errorf("want status %d, got %d", want, resp.StatusCode)
	}
}

func assertBody(tb testing.TB, got, want string) {
	tb.Helper()
	if got != want {
		tb.Errorf("want body %q, got %q", want, got)
	}
}

func assertContains(tb testing.TB, s, substr string) {
	tb.Helper()
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return
		}
	}
	tb.Errorf("want %q to contain %q", s, substr)
}
