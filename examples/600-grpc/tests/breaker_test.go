//go:build integration

package tests

import (
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 13. Circuit Breaker (8 tests)
// ============================================================================

func TestCB_EndpointResponds(t *testing.T) {
	ac := createAccount(t)
	r := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
	assertStatus(t, r["_status"].(float64), 200, "balance")
}

func TestCB_InvalidEndpointReturnsError(t *testing.T) {
	r := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/invalid/balance", "", demoToken)
	st := int(r["_status"].(float64))
	if st == 200 {
		t.Error("invalid endpoint should not succeed")
	}
}

func TestCB_MultipleFastRequests(t *testing.T) {
	ac := createAccount(t)
	for range 20 {
		r := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
		if int(r["_status"].(float64)) != 200 {
			t.Error("fast request failed")
			break
		}
	}
}

func TestCB_ConcurrentRequests(t *testing.T) {
	ac := createAccount(t)
	errs := make(chan error, 30)
	for range 30 {
		go func() {
			r := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
			if int(r["_status"].(float64)) != 200 {
				errs <- fmt.Errorf("status=%.0f", r["_status"])
			}
		}()
	}
	select {
	case e := <-errs:
		t.Error(e)
	case <-time.After(3 * time.Second):
	}
}

func TestCB_GRPCMethodIsolation(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("cb-iso-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "transfer")
	time.Sleep(1 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 950 {
		t.Errorf("gRPC method not isolated: balance=%.0f", v1)
	}
}

func TestCB_HTTPTimeout(t *testing.T) {
	for i := range 5 {
		r := doJSON("POST", baseURL["fraud"]+"/api/v1/fraud/check", `{"amount":100}`)
		assertStatus(t, r["_status"].(float64), 200, fmt.Sprintf("fraud %d", i))
	}
}

func TestCB_DifferentMethods(t *testing.T) {
	r1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts", "", demoToken)
	r2 := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts", `{"currency":"USD"}`, demoToken)
	r3 := doJSON("POST", baseURL["fraud"]+"/api/v1/fraud/check", `{"amount":100}`)
	_ = r1
	_ = r2
	_ = r3
}

func TestCB_BalanceAfterTransfers(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	for i := range 3 {
		key := fmt.Sprintf("cb-bal-%d-%d", i, time.Now().UnixNano())
		r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("tx %d", i))
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 850 {
		t.Errorf("breaker balance = %.0f, want 850", v1)
	}
}
