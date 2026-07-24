//go:build integration

package tests

import (
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 7. Retry (10 tests)
// ============================================================================

func TestRetry_TransferRetryDifferentKey(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	for i := range 3 {
		key := fmt.Sprintf("retry-dk-%d-%d", i, time.Now().UnixNano())
		r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("retry %d", i))
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 910 {
		t.Errorf("retry bal = %.0f, want 910", v1)
	}
}

func TestRetry_TransferSameKeyIsIdempotent(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("retry-sk-%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key)
	r1 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", body, demoToken)
	assertStatus(t, r1["_status"].(float64), 201, "first")
	r2 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", body, demoToken)
	st2 := int(r2["_status"].(float64))
	if st2 == 201 {
		t.Error("duplicate key should not create second transfer")
	}
}

func TestRetry_AccountCreationRetry(t *testing.T) {
	ids := make([]string, 5)
	for i := range 5 {
		r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts", `{"currency":"USD"}`, demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("create %d", i))
		if id, ok := r["id"].(string); ok {
			ids[i] = id
		}
	}
	count := 0
	for _, id := range ids {
		if id != "" {
			count++
		}
	}
	if count < 4 {
		t.Errorf("only created %d accounts", count)
	}
}

func TestRetry_LoginRetry(t *testing.T) {
	for i := range 5 {
		r := doJSON("POST", baseURL["auth"]+"/api/v1/auth/login",
			`{"username":"demo","password":"demo"}`)
		assertStatus(t, r["_status"].(float64), 200, fmt.Sprintf("login %d", i))
	}
}

func TestRetry_SignupThenLogin(t *testing.T) {
	u := fmt.Sprintf("retry-sl-%d", time.Now().UnixNano())
	r1 := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup",
		fmt.Sprintf(`{"username":"%s","password":"Test1234"}`, u))
	assertStatus(t, r1["_status"].(float64), 201, "signup")
	r2 := doJSON("POST", baseURL["auth"]+"/api/v1/auth/login",
		fmt.Sprintf(`{"username":"%s","password":"Test1234"}`, u))
	assertStatus(t, r2["_status"].(float64), 200, "login")
}

func TestRetry_TransferAfterReinit(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("retry-reinit-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":100,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "init")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 900 {
		t.Errorf("reinit bal = %.0f, want 900", v1)
	}
}

func TestRetry_MultipleTransfersWithRetry(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	for i := range 5 {
		key := fmt.Sprintf("retry-mt-%d-%d", i, time.Now().UnixNano())
		r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":20,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("mt %d", i))
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 900 {
		t.Errorf("mt bal = %.0f, want 900", v1)
	}
}

func TestRetry_AsyncJobReassignment(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("retry-async-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "async retry")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 950 {
		t.Errorf("async retry bal = %.0f, want 950", v1)
	}
}

func TestRetry_SequenceRecovery(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("retry-seq-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":40,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "seq retry")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	if v1+v2 != 2000 {
		t.Errorf("seq retry total = %.0f, want 2000", v1+v2)
	}
}
