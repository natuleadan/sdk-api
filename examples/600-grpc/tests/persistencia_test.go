//go:build integration

package tests

import (
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// 10. Persistence (10 tests)
// ============================================================================

func TestPersist_AccountCreated(t *testing.T) {
	r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts", `{"currency":"USD"}`, demoToken)
	assertStatus(t, r["_status"].(float64), 201, "create")
	if r["id"] == nil {
		t.Error("account id missing")
	}
}

func TestPersist_BalanceAfterCreate(t *testing.T) {
	ac := createAccount(t)
	r := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
	assertStatus(t, r["_status"].(float64), 200, "balance")
	bal, _ := r["balance"].(float64)
	if bal != 1000 {
		t.Errorf("balance = %.0f, want 1000", bal)
	}
}

func TestPersist_TransferRecorded(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("pers-tx-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":100,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "transfer")
	if r["transfer_id"] == nil {
		t.Error("transfer_id missing")
	}
}

func TestPersist_BalanceUpdatedAfterAsync(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("pers-bal-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":150,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	v2, _ := b2["balance"].(float64)
	if v1+v2 != 2000 {
		t.Errorf("persistence total = %.0f, want 2000", v1+v2)
	}
}

func TestPersist_MultipleTransfersSum(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("pers-mult-%d", time.Now().UnixNano())
	for i := range 5 {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":20,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	}
	time.Sleep(3 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 != 900 {
		t.Errorf("persist mult = %.0f, want 900", v1)
	}
}

func TestPersist_SenderBalanceDecreased(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("pers-sbd-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":200,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 >= 1000 {
		t.Errorf("sender balance not decreased: %.0f", v1)
	}
}

func TestPersist_ReceiverBalanceIncreased(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("pers-rbi-%d", time.Now().UnixNano())
	doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":200,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	time.Sleep(2 * time.Second)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	v2, _ := b2["balance"].(float64)
	if v2 <= 1000 {
		t.Errorf("receiver balance not increased: %.0f", v2)
	}
}

func TestPersist_MoneyNotLost(t *testing.T) {
	ac1, ac2, ac3 := createAccount(t), createAccount(t), createAccount(t)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	b3 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac3+"/balance", "", demoToken)
	totalBefore, _ := b1["balance"].(float64)
	totalBefore += b2["balance"].(float64)
	totalBefore += b3["balance"].(float64)

	base := fmt.Sprintf("pers-notlost-%d", time.Now().UnixNano())
	for i, pair := range [][2]string{{ac1, ac2}, {ac2, ac3}, {ac3, ac1}} {
		key := fmt.Sprintf("%s-%d", base, i)
		doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, pair[0], pair[1], key), demoToken)
	}
	time.Sleep(3 * time.Second)
	b1a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	b3a := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac3+"/balance", "", demoToken)
	totalAfter, _ := b1a["balance"].(float64)
	totalAfter += b2a["balance"].(float64)
	totalAfter += b3a["balance"].(float64)
	if totalAfter != totalBefore {
		t.Errorf("money lost: %.0f → %.0f", totalBefore, totalAfter)
	}
}

func TestPersist_CrossCurrencyTotal(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	b2 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	total, _ := b1["balance"].(float64)
	total += b2["balance"].(float64)
	if total != 2000 {
		t.Errorf("cross currency total = %.0f, want 2000", total)
	}
}
