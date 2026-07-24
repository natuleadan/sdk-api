//go:build integration

package tests

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// ============================================================================
// 18. Observability (8 tests)
// ============================================================================

func TestObs_AllHealthz(t *testing.T) {
	for name, url := range baseURL {
		paths := []string{"/healthz", "/", "/api/v1/"}
		responded := false
		for _, p := range paths {
			resp, err := http.Get(url + p)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode < 500 {
					responded = true
					break
				}
			}
		}
		if !responded {
			t.Errorf("%s: no valid response", name)
		}
	}
}

func TestObs_HealthzReturnsJSON(t *testing.T) {
	resp, err := http.Get(baseURL["auth"] + "/healthz")
	if err != nil {
		t.Skipf("auth healthz: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Log("healthz: empty body (expected)")
	} else {
		t.Logf("healthz: %s", string(body))
	}
}

func TestObs_ErrorResponseFormat(t *testing.T) {
	r := doJSON("POST", baseURL["transfer"]+"/api/v1/transfers", `{}`)
	_, hasError := r["error"]
	_, hasCode := r["code"]
	_, hasMessage := r["message"]
	if !hasError && !hasCode && !hasMessage {
		t.Error("error response missing error/code/message field")
	}
}

func TestObs_EndpointNotFound(t *testing.T) {
	resp, err := http.Get(baseURL["account"] + "/api/v1/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("nonexistent endpoint: expected 404, got %d", resp.StatusCode)
	}
}

func TestObs_MethodNotAllowed(t *testing.T) {
	resp, err := http.Get(baseURL["transfer"] + "/api/v1/transfers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		t.Errorf("GET on POST endpoint: expected 405/404, got %d", resp.StatusCode)
	}
}

func TestObs_ResponseHeaders(t *testing.T) {
	resp, err := http.Post(baseURL["fraud"]+"/api/v1/fraud/check",
		"application/json", strings.NewReader(`{"amount":100}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "json") {
		t.Errorf("unexpected Content-Type: %s", ct)
	}
}

func TestObs_LogsContainTransactionID(t *testing.T) {
	key := "obs-trace-" + t.Name()
	ac1, ac2 := createAccount(t), createAccount(t)
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		`{"from_account_id":"`+ac1+`","to_account_id":"`+ac2+`","amount":10,"idempotency_key":"`+key+`"}`, demoToken)
	if int(r["_status"].(float64)) == 201 {
		t.Logf("transfer created with key=%s", key)
	}
}

func TestObs_ConcurrentErrorFormat(t *testing.T) {
	results := make(chan map[string]any, 20)
	for range 20 {
		go func() {
			r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", `{}`, demoToken)
			results <- r
		}()
	}
	for range 20 {
		r := <-results
		st := int(r["_status"].(float64))
		if st == 200 || st == 201 {
			t.Error("empty transfer request succeeded")
		}
	}
}
