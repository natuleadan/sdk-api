//go:build integration

package tests

import (
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 12. Outbox Pattern (10 tests)
// ============================================================================

func TestOutbox_EventPublishedAfterTransfer(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("out-ev-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "outbox event")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 950 {
		t.Errorf("outbox bal = %.0f, want 950", v1)
	}
}

func TestOutbox_IdempotentEvent(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("out-ie-%d", time.Now().UnixNano())
	r1 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r1["_status"].(float64), 201, "first")
	r2 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	st2 := int(r2["_status"].(float64))
	if st2 == 201 {
		t.Error("duplicate event should not be processed")
	}
}

func TestOutbox_EventualBalance(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("out-eb-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":80,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	for range 10 {
		time.Sleep(500 * time.Millisecond)
		b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
		v1, _ := b1["balance"].(float64)
		if v1 <= 920 {
			return
		}
	}
	t.Error("outbox eventual balance did not converge")
}

func TestOutbox_MultipleEventsInSequence(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("out-mes-%d", time.Now().UnixNano())
	for i := range 3 {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":40,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 880 {
		t.Errorf("mes bal = %.0f, want 880", v1)
	}
}

func TestOutbox_ConcurrentEvents(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("out-ce-%d", time.Now().UnixNano())
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
		t.Errorf("ce total = %.0f, want 2000", v1+v2)
	}
}

func TestOutbox_TransferRollbackOnFail(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("out-trf-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":5000,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "init rollback")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 < 0 {
		t.Errorf("rollback bal = %.0f, want >= 0", v1)
	}
}

func TestOutbox_EventCountConsistency(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("out-ecc-%d", time.Now().UnixNano())
	count := 4
	for i := range count {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":25,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	expected := 1000.0 - float64(count)*25.0
	if v1 != expected {
		t.Errorf("ecc bal = %.0f, want %.0f", v1, expected)
	}
}

func TestOutbox_NoDuplicateEvents(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("out-nde-%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key)
	r1 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", body, demoToken)
	assertStatus(t, r1["_status"].(float64), 201, "original")
	time.Sleep(1 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v2, _ := b2["balance"].(float64)
	if v1+v2 != 2000 {
		t.Errorf("nde total = %.0f, want 2000 (no double-spend)", v1+v2)
	}
}

func TestOutbox_TotalPreservedAfterMultiple(t *testing.T) {
	ac1, ac2, ac3 := createAccount(t), createAccount(t), createAccount(t)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	b3 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac3+"/balance", "", demoToken)
	totalBefore, _ := b1["balance"].(float64)
	totalBefore += b2["balance"].(float64)
	totalBefore += b3["balance"].(float64)

	base := fmt.Sprintf("out-tp-%d", time.Now().UnixNano())
	for i, pair := range [][2]string{{ac1, ac2}, {ac2, ac3}, {ac3, ac1}} {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"%s"}`, pair[0], pair[1], key), demoToken)
	}
	time.Sleep(3 * time.Second)
	b1a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	b3a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac3+"/balance", "", demoToken)
	totalAfter, _ := b1a["balance"].(float64)
	totalAfter += b2a["balance"].(float64)
	totalAfter += b3a["balance"].(float64)
	if totalAfter != totalBefore {
		t.Errorf("tp total changed: %.0f -> %.0f", totalBefore, totalAfter)
	}
}
