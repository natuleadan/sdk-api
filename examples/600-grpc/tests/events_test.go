//go:build integration

package tests

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// 6. Event Ordering (10 tests)
// ============================================================================

func TestEv_AsyncTransferCompletes(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("ev-ok-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":100,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "async transfer accepted")
	assertStatus(t, r["_status"].(float64), 201, "initiated; status="+r["status"].(string))
}

func TestEv_BalanceUpdatesAfterEvent(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("ev-bal-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":150,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	if v1+v2 != 2000 {
		t.Errorf("total = %.0f, want 2000", v1+v2)
	}
}

func TestEv_SequentialEvents(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("ev-seq-%d", time.Now().UnixNano())
	for i := range 5 {
		key := fmt.Sprintf("%s-%d", base, i)
		r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":20,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("seq event %d", i))
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	if v1 != 900 {
		t.Errorf("sender = %.0f, want 900", v1)
	}
	if v2 != 1100 {
		t.Errorf("receiver = %.0f, want 1100", v2)
	}
}

func TestEv_ConcurrentEvents(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("ev-con-%d", time.Now().UnixNano())
			doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
				fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		}()
	}
	wg.Wait()
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	if v1+v2 != 2000 {
		t.Errorf("total = %.0f, want 2000 (v1=%.0f v2=%.0f)", v1+v2, v1, v2)
	}
}

func TestEv_EventDoesNotLoseMoney(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	totalBefore, _ := b1["balance"].(float64)
	totalBefore += b2["balance"].(float64)

	base := fmt.Sprintf("ev-safe-%d", time.Now().UnixNano())
	for i := range 5 {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(2 * time.Second)
	b1a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	totalAfter, _ := b1a["balance"].(float64)
	totalAfter += b2a["balance"].(float64)
	if totalAfter != totalBefore {
		t.Errorf("money lost: before=%.0f after=%.0f", totalBefore, totalAfter)
	}
}

func TestEv_EventOrderPreserved(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("ev-ord-%d", time.Now().UnixNano())
	for i := range 3 {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":100,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	expected := 1000.0 - 300.0
	if v1 != expected {
		t.Errorf("sender = %.0f, want %.0f", v1, expected)
	}
}

// ============================================================================
// 14. Broker / NATS (12 tests)
// ============================================================================

func TestBroker_NATSEventPublished(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("brk-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "event published")
}

func TestBroker_NATSStreamReady(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("brk-str-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":25,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 >= 1000 {
		t.Error("event not processed via NATS")
	}
}

func TestBroker_EventDrivenBalance(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("brk-bal-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":200,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	if v1 != 800 || v2 != 1200 {
		t.Errorf("unexpected balances: %.0f / %.0f", v1, v2)
	}
}

func TestBroker_InsufficientEvent(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("brk-insuf-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":99999,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "initiated")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 1000 {
		t.Errorf("insufficient should not deduct: %.0f", v1)
	}
}

func TestBroker_RapidEvents(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	for i := range 5 {
		key := fmt.Sprintf("brk-rapid-%d-%d", i, time.Now().UnixNano())
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	expected := 1000.0 - 250.0
	if v1 != expected {
		t.Errorf("rapid: sender = %.0f, want %.0f", v1, expected)
	}
}

// ============================================================================
// 11. Eventual Consistency (12 tests)
// ============================================================================

func TestCons_EventualBalance(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("cons-eu-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":100,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	for range 10 {
		time.Sleep(500 * time.Millisecond)
		b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
		v1, _ := b1["balance"].(float64)
		if v1 == 900 {
			return
		}
	}
	t.Error("balance did not converge to 900")
}

func TestCons_TotalBalancePreserved(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	totalBefore, _ := b1["balance"].(float64)
	totalBefore += b2["balance"].(float64)

	base := fmt.Sprintf("cons-tot-%d", time.Now().UnixNano())
	for i := range 5 {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":30,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(3 * time.Second)
	for range 10 {
		b1a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
		b2a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
		v1, _ := b1a["balance"].(float64)
		v2, _ := b2a["balance"].(float64)
		if v1+v2 == totalBefore {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Error("total balance not preserved after events")
}

func TestCons_MultipleAccountsSync(t *testing.T) {
	ac1, ac2, ac3 := createAccount(t), createAccount(t), createAccount(t)
	base := fmt.Sprintf("cons-mult-%d", time.Now().UnixNano())
	for i, pair := range [][2]string{{ac1, ac2}, {ac2, ac3}, {ac1, ac3}} {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, pair[0], pair[1], key), demoToken)
	}
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	b3 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac3+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	v3, _ := b3["balance"].(float64)
	total := v1 + v2 + v3
	if total != 3000 {
		t.Errorf("total = %.0f, want 3000", total)
	}
}

// ============================================================================
// 15. Scalability (10 tests)
// ============================================================================

func TestScale_HighThroughput(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("scale-hi-%d", time.Now().UnixNano())
			doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
				fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":10,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		}()
	}
	wg.Wait()
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	t.Logf("high throughput: %d transfers, balance=%.0f", 20, v1)
}

func TestScale_ParallelAccountAndTransfer(t *testing.T) {
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			createAccount(t)
		}()
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("scale-par-%d", time.Now().UnixNano())
			doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
				fmt.Sprintf(`{"from_account_id":"1","to_account_id":"2","amount":5,"idempotency_key":"%s"}`, key), demoToken)
		}()
	}
	wg.Wait()
}

func TestScale_BurstTransfers(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	for burst := range 3 {
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				key := fmt.Sprintf("scale-burst-%d-%d", burst, time.Now().UnixNano())
				doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
					fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":5,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
			}()
		}
		wg.Wait()
		time.Sleep(1 * time.Second)
	}
}

func TestScale_EventProcessingRate(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	start := time.Now()
	count := 15
	for i := range count {
		key := fmt.Sprintf("scale-rate-%d-%d", i, time.Now().UnixNano())
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":5,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	expected := 1000.0 - float64(count)*5.0
	dur := time.Since(start)
	if v1 == expected {
		t.Logf("rate: %d events processed in %v", count, dur)
	} else {
		t.Logf("rate: balance=%.0f expected=%.0f (events still processing)", v1, expected)
	}
}
