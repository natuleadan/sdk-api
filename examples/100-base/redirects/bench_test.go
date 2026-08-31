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
	httpPort = "23103"
	baseURL  = "http://localhost:" + httpPort
)

var docker bool

// noFollowClient prevents automatic redirect following so we can assert 3xx + Location.
var noFollowClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func TestMain(m *testing.M) {
	docker = os.Getenv("DOCKER_TEST") == "1"
	if !docker {
		if _, err := exec.LookPath("go"); err != nil {
			fmt.Println("skip: no go compiler")
			os.Exit(0)
		}
	}
	if !docker {
		exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/redirects-svc", "./cmd/").Run()
	}
	os.Exit(m.Run())
}

func TestRedirect_ExactPath(t *testing.T) {
	waitForService(t)
	resp := httpGetNoFollow(t, "/old")
	defer resp.Body.Close()
	assertStatus(t, resp, 302)
	assertLocation(t, resp, "/new")
}

func TestRedirect_Permanent(t *testing.T) {
	waitForService(t)
	resp := httpGetNoFollow(t, "/blog")
	defer resp.Body.Close()
	assertStatus(t, resp, 301)
	assertLocation(t, resp, "/api/posts")
}

func TestRedirect_WildcardForward(t *testing.T) {
	waitForService(t)
	resp := httpGetNoFollow(t, "/old/page1")
	defer resp.Body.Close()
	assertStatus(t, resp, 302)
	assertLocation(t, resp, "/new/page1")
}

func TestRedirect_ParamForward(t *testing.T) {
	waitForService(t)
	resp := httpGetNoFollow(t, "/user/42")
	defer resp.Body.Close()
	assertStatus(t, resp, 301)
	assertLocation(t, resp, "/profile/42")
}

func TestRedirect_QueryPreserved(t *testing.T) {
	waitForService(t)
	resp := httpGetNoFollow(t, "/search?q=hello&page=1")
	defer resp.Body.Close()
	assertStatus(t, resp, 307)
	loc := resp.Header.Get("Location")
	assertContains(t, loc, "/api/search?q=hello")
}

func TestRedirect_308MethodChange(t *testing.T) {
	waitForService(t)
	resp := httpGetNoFollow(t, "/api/v1/data")
	defer resp.Body.Close()
	assertStatus(t, resp, 308)
	assertLocation(t, resp, "/api/v2/data")
}

func TestRedirect_MethodFilter(t *testing.T) {
	waitForService(t)
	resp := httpGetNoFollow(t, "/limited")
	defer resp.Body.Close()
	assertStatus(t, resp, 302)
	assertLocation(t, resp, "/api/limited")
}

func BenchmarkRedirect(b *testing.B) {
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

	b.Run("redirect", func(b *testing.B) {
		b.ResetTimer()
		var wg sync.WaitGroup
		workers := 10
		iterPerWorker := b.N / workers
		for range workers {
			wg.Go(func() {
				for range iterPerWorker {
					resp, err := client.Get(baseURL + "/old")
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
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/redirects-svc", "./cmd/").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build: %w\n%s", err, out)
	}
	return "/tmp/redirects-svc", nil
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

func httpGetNoFollow(tb testing.TB, path string) *http.Response {
	tb.Helper()
	resp, err := noFollowClient.Get(baseURL + path)
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

func assertLocation(tb testing.TB, resp *http.Response, want string) {
	tb.Helper()
	got := resp.Header.Get("Location")
	if got != want {
		tb.Errorf("want Location %q, got %q", want, got)
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
