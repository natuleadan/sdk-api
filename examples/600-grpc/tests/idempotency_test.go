//go:build integration

package tests

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestIdempot_SameKey(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := "ip-sk-" + fmt.Sprintf("%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key)

	r1 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", body, demoToken)
	assertStatus(t, r1["_status"].(float64), 201, "first")

	r2 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", body, demoToken)
	st := int(r2["_status"].(float64))
	if st != 500 && st != 409 {
		t.Errorf("duplicate should fail, got %d", st)
	}
}

func TestIdempot_DifferentKey(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	n := fmt.Sprintf("%d", time.Now().UnixNano())
	b1 := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"dk-%s-1"}`, ac1, ac2, n)
	r1 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", b1, demoToken)
	assertStatus(t, r1["_status"].(float64), 201, "first")
	b2 := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"dk-%s-2"}`, ac1, ac2, n)
	r2 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", b2, demoToken)
	assertStatus(t, r2["_status"].(float64), 201, "second diff key")
}

func TestIdempot_InsufficientFunds(t *testing.T) {
	ac := createAccount(t)
	key := "if-" + fmt.Sprintf("%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":999999,"idempotency_key":"%s"}`, ac, ac, key), demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		time.Sleep(2 * time.Second)
		b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
		v1, _ := b1["balance"].(float64)
		if v1 < 0 {
			t.Errorf("insufficient: balance=%.0f", v1)
		}
	} else {
		t.Logf("insufficient funds: status=%d", st)
	}
}

func TestIdempot_NegativeAmount(t *testing.T) {
	ac := createAccount(t)
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":-50,"idempotency_key":"neg"}`, ac, ac), demoToken)
	assertStatus(t, r["_status"].(float64), 400, "negative")
}

func TestIdempot_ZeroAmount(t *testing.T) {
	ac := createAccount(t)
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":0,"idempotency_key":"zero"}`, ac, ac), demoToken)
	assertStatus(t, r["_status"].(float64), 400, "zero")
}

func TestIdempot_ConcurrentSameKey(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := "ip-con-" + fmt.Sprintf("%d", time.Now().UnixNano())
	var wg sync.WaitGroup
	results := make([]map[string]any, 10)
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":10,"idempotency_key":"%s"}`, ac1, ac2, key)
			results[idx] = doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", b, demoToken)
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, r := range results {
		if int(r["_status"].(float64)) == 201 {
			ok++
		}
	}
	if ok != 1 {
		t.Errorf("expected 1 success, got %d", ok)
	}
}

func TestIdempot_InvalidAccount(t *testing.T) {
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		`{"from_account_id":"999","to_account_id":"888","amount":50,"idempotency_key":"inv"}`, demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Log("invalid account initiated (async)")
	} else {
		t.Logf("invalid account: %d", st)
	}
}

func TestIdempot_SelfTransfer(t *testing.T) {
	ac := createAccount(t)
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":10,"idempotency_key":"self"}`, ac, ac), demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		bal := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
		bv, _ := bal["balance"].(float64)
		if bv != 1000 {
			t.Errorf("self-transfer changed balance from 1000 to %.0f", bv)
		}
	} else {
		t.Logf("self-transfer returned %d (acceptable)", st)
	}
}

func TestIdempot_DeductNoFunds(t *testing.T) {
	ac := createAccount(t)
	key := "nf-" + fmt.Sprintf("%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":5000,"idempotency_key":"%s"}`, ac, ac, key), demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		time.Sleep(2 * time.Second)
		b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
		v1, _ := b1["balance"].(float64)
		if v1 < 0 {
			t.Errorf("deduct no funds: balance=%.0f", v1)
		}
	} else {
		t.Logf("deduct no funds: status=%d", st)
	}
}

func TestIdempot_ReplayAfterSuccess(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := "ip-replay-" + fmt.Sprintf("%d", time.Now().UnixNano())
	b := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":25,"idempotency_key":"%s"}`, ac1, ac2, key)
	r1 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", b, demoToken)
	assertStatus(t, r1["_status"].(float64), 201, "first")
	r2 := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", b, demoToken)
	if int(r2["_status"].(float64)) == 201 {
		t.Error("replay should not succeed")
	}
}
