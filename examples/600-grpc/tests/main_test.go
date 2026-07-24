//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var baseURL = map[string]string{}

func init() {
	mode := os.Getenv("SVC_HOST")
	if mode == "" {
		mode = "localhost"
	}
	ports := map[string]string{
		"auth": "23601", "url": "23602", "file": "23603",
		"ticket": "23604", "account": "23605", "transfer": "23606",
		"fraud": "23607", "receipt": "23608",
	}
	hosts := map[string]string{
		"auth": "auth-svc", "url": "url-svc", "file": "file-svc",
		"ticket": "ticket-svc", "account": "account-svc",
		"transfer": "transfer-svc", "fraud": "fraud-svc", "receipt": "receipt-svc",
	}
	baseURL = make(map[string]string, len(ports))
	for name, port := range ports {
		h := mode
		if mode == "docker" {
			h = hosts[name]
		}
		baseURL[name] = fmt.Sprintf("http://%s:%s", h, port)
	}
}

var demoToken string

func TestMain(m *testing.M) {
	for name := range baseURL {
		waitHTTP(name, 30*time.Second)
	}

	body := doJSON("POST", baseURL["auth"]+"/api/v1/auth/login",
		`{"username":"demo","password":"demo"}`)
	tok, _ := body["token"].(string)
	if tok == "" {
		// Try creating demo user if not seeded
		signup := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup",
			`{"username":"demo","password":"Demo1234"}`)
		_ = signup
		body = doJSON("POST", baseURL["auth"]+"/api/v1/auth/login",
			`{"username":"demo","password":"Demo1234"}`)
		tok, _ = body["token"].(string)
	}
	demoToken = tok
	if demoToken == "" {
		fmt.Println("ERROR: could not login or signup demo user")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// ============================================================================
// Helpers
// ============================================================================

func waitHTTP(svc string, timeout time.Duration) {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL[svc] + "/healthz")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func doJSON(method, url, body string) map[string]any {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"_error": err.Error(), "_status": float64(0)}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(b, &result)
	if result == nil {
		result = map[string]any{}
	}
	result["_status"] = float64(resp.StatusCode)
	return result
}

func doJSONAuth(method, url, body, token string) map[string]any {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"_error": err.Error(), "_status": float64(0)}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(b, &result)
	if result == nil {
		result = map[string]any{}
	}
	result["_status"] = float64(resp.StatusCode)
	return result
}

func assertStatus(t *testing.T, got float64, want int, msg string) {
	t.Helper()
	if int(got) != want {
		t.Errorf("%s: status = %d, want %d", msg, int(got), want)
	}
}

func createAccount(t *testing.T) string {
	t.Helper()
	resp := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts",
		`{"currency":"USD"}`, demoToken)
	assertStatus(t, resp["_status"].(float64), 201, "create account")
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatal("account id is empty")
	}
	return id
}
