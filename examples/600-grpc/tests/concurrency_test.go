//go:build integration

package tests

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConc_RaceDeduct(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	var wg sync.WaitGroup
	ok := 0
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("crd-%d", time.Now().UnixNano())
			r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
				fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":100,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
			if int(r["_status"].(float64)) == 201 {
				ok++
			}
		}()
	}
	wg.Wait()
	time.Sleep(2 * time.Second)
	bal := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	bv, _ := bal["balance"].(float64)
	if bv < 0 || bv > 1000 {
		t.Errorf("balance out of range: %.0f", bv)
	}
}

func TestConc_SequentialTransfers(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("seq-%d", time.Now().UnixNano())
	for i := range 5 {
		key := fmt.Sprintf("%s-%d", base, i)
		r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("seq %d", i))
	}
}

func TestConc_ParallelCreate(t *testing.T) {
	var wg sync.WaitGroup
	ids := make([]string, 10)
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts", `{"currency":"USD"}`, demoToken)
			if id, _ := r["id"].(string); id != "" {
				ids[idx] = id
			}
		}(i)
	}
	wg.Wait()
	c := 0
	for _, id := range ids {
		if id != "" {
			c++
		}
	}
	if c < 8 {
		t.Errorf("created only %d accounts", c)
	}
}

func TestConc_BalanceConsistency(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("cons-%d", time.Now().UnixNano())
			doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
				fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":10,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		}()
	}
	wg.Wait()
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	if v1+v2 != 2000 {
		t.Errorf("total = %.0f, want 2000", v1+v2)
	}
}

func TestConc_ParallelLogin(t *testing.T) {
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doJSON("POST", baseURL["auth"]+"/api/v1/auth/login", `{"username":"demo","password":"demo"}`)
		}()
	}
	wg.Wait()
}

func TestConc_MixedOperations(t *testing.T) {
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(4)
		go func() {
			defer wg.Done()
			doJSON("POST", baseURL["fraud"]+"/api/v1/fraud/check", `{"amount":100}`)
		}()
		go func() {
			defer wg.Done()
			doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts", `{"currency":"USD"}`, demoToken)
		}()
		go func() {
			defer wg.Done()
			doJSONAuth("GET", baseURL["auth"]+"/api/v1/credits/balance", "", demoToken)
		}()
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("mix-%d", time.Now().UnixNano())
			doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
				fmt.Sprintf(`{"from_account_id":"1","to_account_id":"2","amount":5,"idempotency_key":"%s"}`, key), demoToken)
		}()
	}
	wg.Wait()
}

func TestConc_ParallelGRPC(t *testing.T) {
	ac := createAccount(t)
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
		}()
	}
	wg.Wait()
}
