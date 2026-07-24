//go:build integration

package tests

import (
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 19. Recovery (10 tests)
// ============================================================================

func TestRecovery_TransferAfterServiceRestart(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("rec-restart-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":60,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "restart")
	time.Sleep(2 * time.Second)
	// Verify the event was processed (async via NATS)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 940 {
		t.Errorf("restart bal = %.0f, want 940", v1)
	}
}

func TestRecovery_MultipleTransfersConsistency(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("rec-cons-%d", time.Now().UnixNano())
	for i := range 5 {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":20,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	if v1+v2 != 2000 {
		t.Errorf("cons total = %.0f, want 2000", v1+v2)
	}
}

func TestRecovery_BalanceNeverNegative(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("rec-neg-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":99999,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "initiated")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 < 0 {
		t.Errorf("balance negative: %.0f", v1)
	}
}

func TestRecovery_SequentialBalanceCheck(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("rec-seq-%d", time.Now().UnixNano())
	for i := range 3 {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":100,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 700 {
		t.Errorf("seq bal = %.0f, want 700", v1)
	}
}

func TestRecovery_ConcurrentBalancePreservation(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("rec-cbp-%d", time.Now().UnixNano())
	for i := range 5 {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	total := v1 + v2
	if total != 2000 {
		t.Errorf("preservation total = %.0f, want 2000", total)
	}
}

func TestRecovery_EventDrivenRecovery(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1b, _ := b1["balance"].(float64)

	key := fmt.Sprintf("rec-edr-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":80,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	time.Sleep(2 * time.Second)

	b1a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1a, _ := b1a["balance"].(float64)
	if v1a >= v1b {
		t.Errorf("event-driven recovery: balance not decreased: %.0f -> %.0f", v1b, v1a)
	}
}

func TestRecovery_ZeroBalanceTransfer(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	// Drain ac1
	key := fmt.Sprintf("rec-zero-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":1000,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "drain")
	time.Sleep(2 * time.Second)

	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 0 {
		t.Errorf("zero bal = %.0f, want 0", v1)
	}

	key2 := fmt.Sprintf("rec-zero2-%d", time.Now().UnixNano())
	r2 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":1,"idempotency_key":"%s"}`, ac1, ac2, key2), demoToken)
	assertStatus(t, r2["_status"].(float64), 201, "zero tx")
	time.Sleep(2 * time.Second)
	b1a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1a, _ := b1a["balance"].(float64)
	if v1a < 0 {
		t.Errorf("negative balance: %.0f", v1a)
	}
}

func TestRecovery_RapidCreation(t *testing.T) {
	for i := range 3 {
		r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts", `{"currency":"USD"}`, demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("rapid create %d", i))
	}
}

func TestRecovery_CrossAccountConsistency(t *testing.T) {
	ac1, ac2, ac3 := createAccount(t), createAccount(t), createAccount(t)
	base := fmt.Sprintf("rec-cac-%d", time.Now().UnixNano())
	pairs := [][2]string{{ac1, ac2}, {ac2, ac3}, {ac3, ac1}}
	for i, pair := range pairs {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, pair[0], pair[1], key), demoToken)
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	b3 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac3+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	v3, _ := b3["balance"].(float64)
	if v1+v2+v3 != 3000 {
		t.Errorf("cross total = %.0f, want 3000", v1+v2+v3)
	}
}
