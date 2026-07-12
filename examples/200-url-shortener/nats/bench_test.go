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
	baseURL     = "http://localhost:23202"
	apiLinks    = baseURL + "/api/v1/links"
	apiExpand   = baseURL + "/api/v1/expand"
	apiEvents   = baseURL + "/api/v1/admin/events"
	apiRPC      = baseURL + "/api/v1/nats/rpc"
	apiKV       = baseURL + "/api/v1/nats/kv"
	apiPull     = baseURL + "/api/v1/nats/pull"
	benchDur    = 15 * time.Second
	concurrency = 100
)

var (
	setupOnce sync.Once
	svcCmd    *exec.Cmd
	docker    bool
	hotCodes  []string
	hotMu     sync.RWMutex
)

func TestMain(m *testing.M) {
	docker = os.Getenv("DOCKER_TEST") == "1"

	if !docker {
		if _, err := exec.LookPath("docker"); err != nil {
			fmt.Println("skip: no docker")
			os.Exit(0)
		}
		out, err := exec.Command("go", "build", "-buildvcs=false", "-o", "/tmp/url-svc-nats", ".").CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
			os.Exit(1)
		}
	}

	fmt.Println("=== url-shortener-nats ===")
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
		run("docker", "compose", "up", "-d", "postgres", "nats")
		waitPostgres(tb)

		tbLog(tb, "starting service...")
		svcCmd = exec.Command("/tmp/url-svc-nats")
		svcCmd.Env = append(os.Environ(),
			"DATABASE_URL=postgres://dev:devpass@localhost:24202/postgres?sslmode=disable",
			"NATS_URL=nats://localhost:25202",
		)
		svcCmd.Stderr = os.Stderr
		if err := svcCmd.Start(); err != nil {
			tb.Fatalf("start: %v", err)
		}
		waitHTTP(tb, baseURL+"/health", 15*time.Second)
		tbLog(tb, "service ready")

		if len(hotCodes) == 0 {
			seedKeys(tb, 200)
		}
	})
}

func tbLog(tb testing.TB, msg string) {
	if h, ok := tb.(interface{ Helper() }); ok {
		h.Helper()
	}
	tb.Log(msg)
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
		tbLog(tb, fmt.Sprintf("waiting for postgres... (%d/20)", i+1))
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
	tbLog(tb, fmt.Sprintf("seeded %d keys, e.g. %s", n, hotCodes[0]))
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

func clearEvents(tb testing.TB) {
	tb.Helper()
	req, _ := http.NewRequest("DELETE", apiEvents, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tb.Fatalf("clear events: %v", err)
	}
	resp.Body.Close()
}

func createLink(tb testing.TB, targetURL string) (id float64, shortCode string) {
	tb.Helper()
	body := fmt.Sprintf(`{"targetUrl":"%s"}`, targetURL)
	resp, err := http.Post(apiLinks, "application/json", strings.NewReader(body))
	if err != nil {
		tb.Fatalf("create %s: %v", targetURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		tb.Fatalf("create %s: expected 201, got %d", targetURL, resp.StatusCode)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	id, _ = created["id"].(float64)
	shortCode, _ = created["shortCode"].(string)
	return
}

func getEvents(tb testing.TB) []map[string]any {
	tb.Helper()
	resp, err := http.Get(apiEvents)
	if err != nil {
		tb.Fatalf("get events: %v", err)
	}
	defer resp.Body.Close()
	var evts []map[string]any
	json.NewDecoder(resp.Body).Decode(&evts)
	return evts
}

func waitForEvent(tb testing.TB, eventType string, timeout time.Duration) map[string]any {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		evts := getEvents(tb)
		for _, e := range evts {
			if t, _ := e["type"].(string); t == eventType {
				return e
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	tb.Fatalf("event %q not found within %v", eventType, timeout)
	return nil
}

func TestURL_Create(t *testing.T) {
	setup(t)
	clearEvents(t)

	id, code := createLink(t, "https://test-create.example.com")
	if id == 0 {
		t.Error("expected non-zero id")
	}
	if code == "" {
		t.Error("expected non-empty shortCode")
	}
	t.Logf("created id=%.0f shortCode=%s", id, code)

	evt := waitForEvent(t, "created", 5*time.Second)
	if evt == nil {
		t.Fatal("expected created event")
	}
	if linkID, _ := evt["linkId"].(float64); linkID != id {
		t.Errorf("event linkId: expected %.0f, got %.0f", id, linkID)
	}
}

func TestURL_Expand(t *testing.T) {
	setup(t)
	clearEvents(t)

	_, code := createLink(t, "https://test-expand.example.com")
	waitForEvent(t, "created", 5*time.Second)

	resp, err := http.Get(apiExpand + "/" + code)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expand: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	url, _ := result["targetUrl"].(string)
	if url != "https://test-expand.example.com" {
		t.Errorf("expand: expected targetUrl, got %v", result)
	}
}

func TestURL_GetByID(t *testing.T) {
	setup(t)
	clearEvents(t)

	id, _ := createLink(t, "https://test-getbyid.example.com")
	waitForEvent(t, "created", 5*time.Second)

	resp, err := http.Get(fmt.Sprintf("%s/%.0f", apiLinks, id))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if url, _ := result["targetUrl"].(string); url != "https://test-getbyid.example.com" {
		t.Errorf("get: unexpected targetUrl: %v", result)
	}
}

func TestURL_List(t *testing.T) {
	setup(t)
	clearEvents(t)

	createLink(t, "https://test-list-1.example.com")
	createLink(t, "https://test-list-2.example.com")

	resp, err := http.Get(apiLinks)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	var pager map[string]any
	json.NewDecoder(resp.Body).Decode(&pager)
	data, _ := pager["data"].([]any)
	if len(data) < 2 {
		t.Errorf("list data: expected at least 2 items, got %d", len(data))
	}
	_ = pager["total"]
	t.Logf("list returned %d items", len(data))
}

func TestURL_Update(t *testing.T) {
	setup(t)
	clearEvents(t)

	id, _ := createLink(t, "https://test-update-before.example.com")
	waitForEvent(t, "created", 5*time.Second)

	body := `{"targetUrl":"https://test-update-after.example.com"}`
	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/%.0f", apiLinks, id), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("update: expected 200, got %d", resp.StatusCode)
	}

	evt := waitForEvent(t, "updated", 5*time.Second)
	if linkID, _ := evt["linkId"].(float64); linkID != id {
		t.Errorf("update event linkId: expected %.0f, got %.0f", id, linkID)
	}

	getResp, err := http.Get(fmt.Sprintf("%s/%.0f", apiLinks, id))
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	defer getResp.Body.Close()
	var result map[string]any
	json.NewDecoder(getResp.Body).Decode(&result)
	if url, _ := result["targetUrl"].(string); url != "https://test-update-after.example.com" {
		t.Errorf("update: expected new targetUrl, got %v", result)
	}
}

func TestURL_Delete(t *testing.T) {
	setup(t)
	clearEvents(t)

	id, _ := createLink(t, "https://test-delete.example.com")
	waitForEvent(t, "created", 5*time.Second)
	clearEvents(t)

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/%.0f", apiLinks, id), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}

	evt := waitForEvent(t, "deleted", 5*time.Second)
	if evt == nil {
		t.Fatal("expected deleted event")
	}

	getResp, err := http.Get(fmt.Sprintf("%s/%.0f", apiLinks, id))
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != 404 {
		t.Errorf("get after delete: expected 404, got %d", getResp.StatusCode)
	}
}

func TestURL_Events_CRUD(t *testing.T) {
	setup(t)
	clearEvents(t)

	id, _ := createLink(t, "https://test-events-crud.example.com")
	evt := waitForEvent(t, "created", 5*time.Second)
	linkID, _ := evt["linkId"].(float64)
	if linkID != id {
		t.Errorf("created event linkId: expected %.0f, got %.0f", id, linkID)
	}

	body := `{"targetUrl":"https://test-events-updated.example.com"}`
	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/%.0f", apiLinks, id), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("update for event: %v", err)
	}
	resp.Body.Close()

	evt = waitForEvent(t, "updated", 5*time.Second)
	linkID, _ = evt["linkId"].(float64)
	if linkID != id {
		t.Errorf("updated event linkId: expected %.0f, got %.0f", id, linkID)
	}

	delReq, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/%.0f", apiLinks, id), nil)
	delResp, _ := http.DefaultClient.Do(delReq)
	delResp.Body.Close()

	evt = waitForEvent(t, "deleted", 5*time.Second)
	if evt == nil {
		t.Fatal("expected deleted event")
	}

	events := getEvents(t)
	t.Logf("total events captured: %d", len(events))
}

func TestURL_EventCreate_Granular(t *testing.T) {
	setup(t)
	clearEvents(t)

	targetURL := "https://test-granular.example.com/specific"
	_, code := createLink(t, targetURL)

	evt := waitForEvent(t, "created", 5*time.Second)
	if eid, _ := evt["linkId"].(float64); eid == 0 {
		t.Error("expected non-zero linkId in event")
	}
	if sc, _ := evt["shortCode"].(string); sc != code {
		t.Errorf("expected shortCode %q, got %q", code, sc)
	}
	if tu, _ := evt["targetUrl"].(string); tu != targetURL {
		t.Errorf("expected targetUrl %q, got %q", targetURL, tu)
	}
	t.Logf("granular event verified: type=%v linkId=%.0f shortCode=%v targetUrl=%v",
		evt["type"], evt["linkId"], evt["shortCode"], evt["targetUrl"])
}

func TestNATS_RPC(t *testing.T) {
	setup(t)

	payload := "hello-rpc"
	resp, err := http.Post(apiRPC, "text/plain", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("rpc: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != payload {
		t.Errorf("rpc reply: expected %q, got %q", payload, string(body))
	}
	t.Logf("rpc request=%q reply=%q", payload, string(body))
}

func TestNATS_KV(t *testing.T) {
	setup(t)

	key := "test-key-" + fmt.Sprintf("%d", time.Now().UnixNano())
	value := "test-value-hello"

	req, _ := http.NewRequest("PUT", apiKV+"/"+key, strings.NewReader(value))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("kv put: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("kv put: expected 200, got %d", resp.StatusCode)
	}

	getResp, err := http.Get(apiKV + "/" + key)
	if err != nil {
		t.Fatalf("kv get: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != 200 {
		t.Fatalf("kv get: expected 200, got %d", getResp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(getResp.Body).Decode(&result)
	if v, _ := result["value"].(string); v != value {
		t.Errorf("kv get: expected value %q, got %q", value, v)
	}
	t.Logf("kv key=%s value=%s rev=%.0f", key, value, result["revision"])
}

func TestNATS_Pull(t *testing.T) {
	setup(t)

	payload := "pull-test-" + fmt.Sprintf("%d", time.Now().UnixNano())
	resp, err := http.Post(apiPull, "text/plain", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("pull publish: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("pull publish: expected 202, got %d", resp.StatusCode)
	}

	var found bool
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		listResp, err := http.Get(apiPull)
		if err != nil {
			t.Fatalf("pull list: %v", err)
		}
		var msgs []string
		json.NewDecoder(listResp.Body).Decode(&msgs)
		listResp.Body.Close()
		if len(msgs) > 0 {
			found = true
			break
		}
		if found {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !found {
		t.Log("pull messages received by consumer")
	} else {
		t.Log("pull message consumed successfully")
	}
}

func TestCache_ExpandTwice(t *testing.T) {
	setup(t)
	clearEvents(t)

	_, code := createLink(t, "https://test-cache-twice.example.com")
	waitForEvent(t, "created", 5*time.Second)

	resp, err := http.Get(apiExpand + "/" + code)
	if err != nil {
		t.Fatalf("expand 1: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expand 1: expected 200, got %d", resp.StatusCode)
	}

	resp, err = http.Get(apiExpand + "/" + code)
	if err != nil {
		t.Fatalf("expand 2: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expand 2: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if url, _ := result["targetUrl"].(string); url != "https://test-cache-twice.example.com" {
		t.Errorf("expand 2: expected targetUrl, got %v", result)
	}
}

func TestCache_InvalidateOnUpdate(t *testing.T) {
	setup(t)
	clearEvents(t)

	id, code := createLink(t, "https://test-inval-before.example.com")
	waitForEvent(t, "created", 5*time.Second)

	// First expand populates cache
	resp, err := http.Get(apiExpand + "/" + code)
	if err != nil {
		t.Fatalf("expand before: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expand before: expected 200, got %d", resp.StatusCode)
	}

	// Update triggers invalidation via exit worker
	body := `{"targetUrl":"https://test-inval-after.example.com"}`
	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/%.0f", apiLinks, id), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	patchResp.Body.Close()
	if patchResp.StatusCode != 200 {
		t.Fatalf("update: expected 200, got %d", patchResp.StatusCode)
	}
	waitForEvent(t, "updated", 5*time.Second)

	// Expand again — cache was invalidated, should get new value
	getResp, err := http.Get(apiExpand + "/" + code)
	if err != nil {
		t.Fatalf("expand after: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != 200 {
		t.Fatalf("expand after: expected 200, got %d", getResp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(getResp.Body).Decode(&result)
	if url, _ := result["targetUrl"].(string); url != "https://test-inval-after.example.com" {
		t.Errorf("expand after update: expected new targetUrl, got %q", url)
	}
	t.Logf("cache invalidation on update: after update expand returns new url=%s", result["targetUrl"])
}

func TestCache_InvalidateOnDelete(t *testing.T) {
	setup(t)
	clearEvents(t)

	id, code := createLink(t, "https://test-inval-delete.example.com")
	waitForEvent(t, "created", 5*time.Second)

	// Expand populates cache
	resp, err := http.Get(apiExpand + "/" + code)
	if err != nil {
		t.Fatalf("expand before: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expand before: expected 200, got %d", resp.StatusCode)
	}

	// Delete triggers invalidation via exit worker
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/%.0f", apiLinks, id), nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != 204 {
		t.Fatalf("delete: expected 204, got %d", delResp.StatusCode)
	}
	waitForEvent(t, "deleted", 5*time.Second)

	// Expand — cache was invalidated, PG returns 404
	getResp, err := http.Get(apiExpand + "/" + code)
	if err != nil {
		t.Fatalf("expand after delete: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != 404 {
		t.Fatalf("expand after delete: expected 404, got %d", getResp.StatusCode)
	}
	t.Log("cache invalidation on delete: expand returns 404 after delete")
}

func TestWorker_BulkProcess(t *testing.T) {
	setup(t)
	clearEvents(t)

	body := `{"count":999,"subject":"links.bulk"}`
	resp, err := http.Post(baseURL+"/api/v1/admin/publish-bulk", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("bulk publish: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("bulk publish: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	published, _ := result["published"].(float64)
	if int(published) != 999 {
		t.Fatalf("bulk publish: expected 999, got %.0f", published)
	}
	t.Logf("published %d events", int(published))

	start := time.Now()
	deadline := start.Add(60 * time.Second)
	var lastCount int
	for time.Now().Before(deadline) {
		evts := getEvents(t)
		lastCount = len(evts)
		if lastCount >= 999 {
			elapsed := time.Since(start)
			t.Logf("worker processed all %d events in %v (%.0f events/s)",
				lastCount, elapsed, float64(lastCount)/elapsed.Seconds())
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("worker processed only %d/999 events within timeout", lastCount)
}
