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
	usersPort    = 13001
	productsPort = 13002
	usersBase    = "http://localhost:13001"
	prodsBase    = "http://localhost:13002"
	usersAPI     = usersBase + "/api/v1/users"
	prodsAPI     = prodsBase + "/api/v1/products"
	msConcur     = 100
	msBenchDur   = 10 * time.Second
)

var (
	msSetupOnce     sync.Once
	usersCmd        *exec.Cmd
	productsCmd     *exec.Cmd
	usersHotIDs     []int64
	usersHotIDMu    sync.RWMutex
	prodsHotIDs     []int64
	prodsHotIDMu    sync.RWMutex
	dockerMS        bool
)

func TestMain(m *testing.M) {
	dockerMS = os.Getenv("DOCKER_TEST") == "1"

	if !dockerMS {
		if _, err := exec.LookPath("docker"); err != nil {
			fmt.Println("docker not found, skipping integration tests")
			os.Exit(0)
		}
		var err error
		_, err = buildUsers()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build users: %v\n", err)
			os.Exit(1)
		}
		_, err = buildProducts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build products: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("=== auth-none-microservices ===")
	code := m.Run()

	if !dockerMS {
		teardownMS()
	}
	os.Exit(code)
}

func buildUsers() (string, error) {
	path := fmt.Sprintf("/tmp/auth-ms-users-%d", time.Now().UnixNano())
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", path, ".").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build: %w\n%s", err, out)
	}
	return path, nil
}

func buildProducts() (string, error) {
	path := fmt.Sprintf("/tmp/auth-ms-products-%d", time.Now().UnixNano())
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", path, ".")
	cmd.Dir = "../products-service"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build: %w\n%s", err, out)
	}
	return path, nil
}

func setupMS(tb testing.TB) {
	if dockerMS {
		setupMSDocker(tb)
		return
	}
	msSetupOnce.Do(func() {
		tb.Log("starting postgres...")
		msRun(tb, "docker", "compose", "-f", "../docker-compose.yml", "down", "-t", "2", "-v")
		msRun(tb, "docker", "compose", "-f", "../docker-compose.yml", "up", "-d")
		waitDockerPG(tb, "none-ms-pg")

		dbURL := "postgres://postgres:postgres@localhost:15433/postgres?sslmode=disable"
		tb.Log("starting users-service...")
		usersCmd = exec.Command("/tmp/auth-ms-users-svc")
		usersCmd.Env = append(os.Environ(), "DATABASE_URL="+dbURL)
		usersCmd.Dir = "."
		usersCmd.Stderr = os.Stderr
		if err := usersCmd.Start(); err != nil {
			tb.Fatalf("start users: %v", err)
		}
		waitHTTP(tb, usersBase+"/health", 15*time.Second)

		tb.Log("starting products-service...")
		productsCmd = exec.Command("/tmp/auth-ms-products-svc")
		productsCmd.Env = append(os.Environ(), "DATABASE_URL="+dbURL)
		productsCmd.Dir = "../products-service"
		productsCmd.Stderr = os.Stderr
		if err := productsCmd.Start(); err != nil {
			tb.Fatalf("start products: %v", err)
		}
		waitHTTP(tb, prodsBase+"/health", 15*time.Second)

		tb.Log("seeding data...")
		seedUsers(tb, 15)
		seedProducts(tb, 15)
		tb.Log("both services ready")
	})
}

func setupMSDocker(tb testing.TB) {
	msSetupOnce.Do(func() {
		waitHTTP(tb, usersBase+"/health", 30*time.Second)
		waitHTTP(tb, prodsBase+"/health", 5*time.Second)
		tb.Log("both services ready (Docker)")
		seedUsers(tb, 15)
		seedProducts(tb, 15)
	})
}

func seedUsers(tb testing.TB, n int) {
	for i := range n {
		body := fmt.Sprintf(`{"name":"User %d","email":"user%d@test.com"}`, i, i)
		resp, err := http.Post(usersAPI, "application/json", strings.NewReader(body))
		if err != nil {
			tb.Fatalf("seed user %d: %v", i, err)
		}
		if resp.StatusCode != 201 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			tb.Fatalf("seed user %d status=%d body=%s", i, resp.StatusCode, string(b))
		}
		var created map[string]any
		json.NewDecoder(resp.Body).Decode(&created)
		resp.Body.Close()
		if id, ok := created["id"].(float64); ok {
			usersHotIDMu.Lock()
			usersHotIDs = append(usersHotIDs, int64(id))
			usersHotIDMu.Unlock()
		}
	}
}

func seedProducts(tb testing.TB, n int) {
	for i := range n {
		body := fmt.Sprintf(`{"name":"Product %d","price":%.2f}`, i, float64(i)*10.0)
		resp, err := http.Post(prodsAPI, "application/json", strings.NewReader(body))
		if err != nil {
			tb.Fatalf("seed product %d: %v", i, err)
		}
		if resp.StatusCode != 201 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			tb.Fatalf("seed product %d status=%d body=%s", i, resp.StatusCode, string(b))
		}
		var created map[string]any
		json.NewDecoder(resp.Body).Decode(&created)
		resp.Body.Close()
		if id, ok := created["id"].(float64); ok {
			prodsHotIDMu.Lock()
			prodsHotIDs = append(prodsHotIDs, int64(id))
			prodsHotIDMu.Unlock()
		}
	}
}

func teardownMS() {
	for _, cmd := range []*exec.Cmd{usersCmd, productsCmd} {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt)
			cmd.Wait()
		}
	}
	msRun(nil, "docker", "compose", "-f", "../docker-compose.yml", "down", "-t", "2", "-v")
}

func waitDockerPG(tb testing.TB, container string) {
	for i := range 20 {
		if msRun(nil, "docker", "exec", container, "pg_isready", "-U", "postgres") == nil {
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
	tb.Fatalf("%s not ready after %v", url, timeout)
}

func msRun(tb testing.TB, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	if tb != nil {
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// --- Functional tests ---

func TestNoneMS_UsersCRUD(t *testing.T) {
	setupMS(t)
	t.Run("users create returns 201", func(t *testing.T) {
		resp, err := http.Post(usersAPI, "application/json",
			strings.NewReader(`{"name":"test-user","email":"test@test.com"}`))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})
	t.Run("users list returns 200", func(t *testing.T) {
		resp, err := http.Get(usersAPI)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestNoneMS_ProductsCRUD(t *testing.T) {
	setupMS(t)
	t.Run("products create returns 201", func(t *testing.T) {
		resp, err := http.Post(prodsAPI, "application/json",
			strings.NewReader(`{"name":"test-prod","price":9.99}`))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})
	t.Run("products list returns 200", func(t *testing.T) {
		resp, err := http.Get(prodsAPI)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestNoneMS_NoAuthRequired(t *testing.T) {
	setupMS(t)
	t.Run("users without auth header works", func(t *testing.T) {
		resp, err := http.Get(usersAPI)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("expected 200 without auth, got %d", resp.StatusCode)
		}
	})
	t.Run("products without auth header works", func(t *testing.T) {
		resp, err := http.Get(prodsAPI)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("expected 200 without auth, got %d", resp.StatusCode)
		}
	})
	t.Run("both services with fake JWT still work", func(t *testing.T) {
		fakeJWT := "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.fake"
		for _, url := range []string{usersAPI, prodsAPI} {
			req, _ := http.NewRequest("GET", url, nil)
			req.Header.Set("Authorization", fakeJWT)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Errorf("%s: expected 200 with fake JWT, got %d", url, resp.StatusCode)
			}
		}
	})
}

func TestNoneMS_ConcurrentBothServices(t *testing.T) {
	setupMS(t)
	var ok, fail atomic.Int64
	var wg sync.WaitGroup
	workers := 25
	iters := 20
	for range workers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range iters {
				resp, err := http.Get(usersAPI)
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
		go func() {
			defer wg.Done()
			for range iters {
				resp, err := http.Get(prodsAPI)
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
		t.Errorf("concurrent: %d failures out of %d", fail.Load(), ok.Load()+fail.Load())
	}
	t.Logf("concurrent: %d ok, %d fail", ok.Load(), fail.Load())
}

func BenchmarkNoneMS_UsersRPS(b *testing.B) {
	setupMS(b)
	transport := &http.Transport{
		MaxIdleConns:        msConcur,
		MaxConnsPerHost:     msConcur,
		MaxIdleConnsPerHost: msConcur,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	var total, errs atomic.Int64
	done := make(chan struct{})
	start := time.Now()
	for range msConcur {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					resp, err := client.Get(usersAPI)
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
	time.Sleep(msBenchDur)
	close(done)
	rps := float64(total.Load()) / time.Since(start).Seconds()
	b.ReportMetric(rps, "req/s")
	b.Logf("=== Users GET RPS ===\nDocker: %v\nRPS: %.0f\nErrors: %d", dockerMS, rps, errs.Load())
	transport.CloseIdleConnections()
}

func BenchmarkNoneMS_ProductsRPS(b *testing.B) {
	setupMS(b)
	transport := &http.Transport{
		MaxIdleConns:        msConcur,
		MaxConnsPerHost:     msConcur,
		MaxIdleConnsPerHost: msConcur,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	var total, errs atomic.Int64
	done := make(chan struct{})
	start := time.Now()
	for range msConcur {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					resp, err := client.Get(prodsAPI)
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
	time.Sleep(msBenchDur)
	close(done)
	rps := float64(total.Load()) / time.Since(start).Seconds()
	b.ReportMetric(rps, "req/s")
	b.Logf("=== Products GET RPS ===\nDocker: %v\nRPS: %.0f\nErrors: %d", dockerMS, rps, errs.Load())
	transport.CloseIdleConnections()
}
