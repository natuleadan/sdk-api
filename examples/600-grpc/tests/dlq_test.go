//go:build integration

package tests

import (
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 8. Dead Letter Queue (8 tests)
// ============================================================================

func TestDLQ_InvalidTransferNak(t *testing.T) {
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		`{"from_account_id":"99999","to_account_id":"99998","amount":50,"idempotency_key":"dlq-inv"}`, demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Log("invalid transfer initiated (async)")
	} else if st >= 400 {
		t.Logf("invalid transfer rejected: %d", st)
	}
}

func TestDLQ_InsufficientFundsNak(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("dlq-insuf-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":99999,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "initiated")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 1000 {
		t.Errorf("insufficient funds should not deduct: %.0f", v1)
	}
}

func TestDLQ_ErrorResponseFormat(t *testing.T) {
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", `{}`, demoToken)
	_, hasErr := r["error"]
	if !hasErr {
		t.Error("error response missing error field")
	}
}

func TestDLQ_InvalidJSONBody(t *testing.T) {
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", `not json`, demoToken)
	st := int(r["_status"].(float64))
	if st != 400 && st != 500 {
		t.Errorf("invalid JSON: expected 400/500, got %d", st)
	}
}

func TestDLQ_EmptyTransfer(t *testing.T) {
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", `{}`, demoToken)
	assertStatus(t, r["_status"].(float64), 400, "empty transfer")
}

func TestDLQ_NegativeAmountTransfer(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("dlq-neg-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":-50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 400, "negative amount")
}

func TestDLQ_MissingSenderAccount(t *testing.T) {
	ac2 := createAccount(t)
	key := fmt.Sprintf("dlq-msa-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac2, key), demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Log("empty sender accepted (will fail async)")
	} else if st >= 400 {
		t.Logf("empty sender rejected: %d", st)
	}
}

func TestDLQ_MissingBothAccounts(t *testing.T) {
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		`{"from_account_id":"","to_account_id":"","amount":50,"idempotency_key":"dlq-mba"}`, demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Error("missing both accounts should not succeed")
	}
}
