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
	httpPort = "23100"
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
		exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/healthz-svc", "./cmd/").Run()
	}
	os.Exit(m.Run())
}

func TestHealthz_OK(t *testing.T) {
	if !docker {
		cmd := exec.Command("/tmp/healthz-svc")
		cmd.Env = append(os.Environ(), "PORT="+httpPort)
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		defer cmd.Process.Kill()
		waitHTTP(t, baseURL+"/healthz", 10*time.Second)
	} else {
		waitHTTP(t, baseURL+"/healthz", 30*time.Second)
	}

	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("expected body 'OK', got '%s'", string(body))
	}
	t.Log("healthz OK")
}

func buildService() (string, error) {
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/healthz-svc", ".").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build: %w\n%s", err, out)
	}
	return "/tmp/healthz-svc", nil
}

func startService(b *testing.B, path string, raw bool) *exec.Cmd {
	cmd := exec.Command(path)
	env := append(os.Environ(), "PORT="+httpPort)
	if raw {
		env = append(env, "RAW=1")
	}
	cmd.Env = env
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		b.Fatalf("start: %v", err)
	}
	return cmd
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
		time.Sleep(200 * time.Millisecond)
	}
	tb.Fatalf("service not ready after %v", timeout)
}

func BenchmarkHealthzRaw(b *testing.B) {
	benchmarkHealthz(b, true)
}

func BenchmarkHealthzSDK(b *testing.B) {
	benchmarkHealthz(b, false)
}

func benchmarkHealthz(b *testing.B, raw bool) {
	svcPath, err := buildService()
	if err != nil {
		b.Fatalf("build: %v", err)
	}

	cmd := startService(b, svcPath, raw)
	defer cmd.Process.Kill()

	waitHTTP(b, baseURL+"/healthz", 10*time.Second)

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxConnsPerHost:     100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	label := "SDK"
	if raw {
		label = "raw Fiber"
	}

	b.Run(label, func(b *testing.B) {
		b.ResetTimer()

		var wg sync.WaitGroup
		workers := 10
		iterPerWorker := b.N / workers

		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < iterPerWorker; i++ {
					resp, err := client.Get(baseURL + "/healthz")
					if err != nil {
						b.Errorf("request failed: %v", err)
						return
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}()
		}
		wg.Wait()
	})

	transport.CloseIdleConnections()
}
