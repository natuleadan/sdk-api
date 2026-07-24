//go:build integration

package tests

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// 9. Timeouts (8 tests)
// ============================================================================

func TestTime_APICallCompletes(t *testing.T) {
	start := time.Now()
	r := doJSON("POST", baseURL["fraud"]+"/api/v1/fraud/check", `{"amount":100}`)
	dur := time.Since(start)
	if dur > 5*time.Second {
		t.Errorf("slow response: %v", dur)
	}
	assertStatus(t, r["_status"].(float64), 200, "fraud")
}

func TestTime_ConcurrentCalls(t *testing.T) {
	start := time.Now()
	done := make(chan bool, 10)
	for range 10 {
		go func() {
			doJSON("POST", baseURL["fraud"]+"/api/v1/fraud/check", `{"amount":100}`)
			done <- true
		}()
	}
	for range 10 {
		<-done
	}
	if time.Since(start) > 10*time.Second {
		t.Error("concurrent calls too slow")
	}
}

func TestTime_SlowEndpoint(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL["account"] + "/healthz")
	if err == nil {
		resp.Body.Close()
	}
}

func TestTime_CancelledContext(t *testing.T) {
	req, _ := http.NewRequest("POST", baseURL["transfer"]+"/api/v1/transfers",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("cancelled request: %v", err)
		return
	}
	resp.Body.Close()
}

func TestTime_TransferWithDelay(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	start := time.Now()
	key := fmt.Sprintf("tim-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	dur := time.Since(start)
	if dur > 10*time.Second {
		t.Errorf("transfer too slow: %v", dur)
	}
	assertStatus(t, r["_status"].(float64), 201, "timed transfer")
}

func TestTime_MultipleRapidTransfers(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	start := time.Now()
	for i := range 3 {
		key := fmt.Sprintf("rapid-%d-%d", i, time.Now().UnixNano())
		r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":10,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("rapid %d", i))
	}
	if time.Since(start) > 30*time.Second {
		t.Error("rapid transfers too slow")
	}
}

func TestTime_AuthLoginLatency(t *testing.T) {
	start := time.Now()
	for range 5 {
		doJSON("POST", baseURL["auth"]+"/api/v1/auth/login",
			`{"username":"demo","password":"demo"}`)
	}
	avg := time.Since(start) / 5
	if avg > 2*time.Second {
		t.Errorf("auth login too slow: avg %v", avg)
	}
}

func TestTime_BalanceQueryLatency(t *testing.T) {
	ac := createAccount(t)
	start := time.Now()
	for range 10 {
		doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
	}
	avg := time.Since(start) / 10
	if avg > 1*time.Second {
		t.Errorf("balance query too slow: avg %v", avg)
	}
}
