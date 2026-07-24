//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// 1-4. Serialization, Contracts, API (38 tests)
// ============================================================================

func TestSer_MarshalJSON_Valid(t *testing.T) {
	r := doJSONAuth("GET", baseURL["auth"]+"/api/v1/credits/balance", "", demoToken)
	if r["_status"].(float64) != 200 {
		t.Fatalf("balance: %v", r["_status"])
	}
	_, ok := r["credits"].(float64)
	if !ok {
		t.Error("credits field missing or wrong type")
	}
}

func TestSer_UnmarshalJSON_Login(t *testing.T) {
	r := doJSON("POST", baseURL["auth"]+"/api/v1/auth/login",
		`{"username":"demo","password":"demo"}`)
	assertStatus(t, r["_status"].(float64), 200, "login")
	tok, ok := r["token"].(string)
	if !ok || len(tok) < 20 {
		t.Error("token field missing or invalid")
	}
}

func TestSer_InvalidJSON_Body(t *testing.T) {
	body := `not json`
	req, _ := http.NewRequest("POST", baseURL["auth"]+"/api/v1/auth/signup",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Errorf("invalid JSON should error, got %d", resp.StatusCode)
	}
}

func TestSer_InvalidJSON_Transfer(t *testing.T) {
	body := `{broken json`
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", body, demoToken)
	st := int(r["_status"].(float64))
	if st != 400 && st != 500 {
		t.Errorf("invalid JSON should error, got %d", st)
	}
}

func TestSer_EmptyBody(t *testing.T) {
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", "", demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Error("empty body should not succeed")
	}
}

func TestSer_MissingRequiredField(t *testing.T) {
	r := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup", `{"username":"only"}`)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Error("missing password should not create user")
	}
}

func TestSer_ExtraField(t *testing.T) {
	r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts",
		`{"currency":"USD","extra":"should be ignored"}`, demoToken)
	assertStatus(t, r["_status"].(float64), 201, "extra field")
}

func TestSer_InvalidFieldType(t *testing.T) {
	r := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup",
		`{"username":"demo","password":123}`)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Error("numeric password should not be accepted")
	}
}

func TestSer_LongPayload(t *testing.T) {
	long := strings.Repeat("A", 10000)
	r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts",
		fmt.Sprintf(`{"currency":"%s"}`, long), demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Log("long payload accepted")
	}
}

func TestSer_SpecialChars(t *testing.T) {
	u := fmt.Sprintf("sp-user-%d", time.Now().UnixNano())
	r := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup",
		fmt.Sprintf(`{"username":"%s","password":"Test!@#123"}`, u))
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Log("special chars password OK")
	} else if st == 400 {
		t.Log("special chars rejected")
	}
}

func TestSer_UnicodePayload(t *testing.T) {
	u := fmt.Sprintf("uni-%d", time.Now().UnixNano())
	r := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup",
		fmt.Sprintf(`{"username":"%s","password":"Passñ123"}`, u))
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Log("unicode password accepted")
	}
}

func TestSer_AccountResponseSchema(t *testing.T) {
	r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts",
		`{"currency":"EUR"}`, demoToken)
	assertStatus(t, r["_status"].(float64), 201, "create account")
	if r["id"] == nil || r["balance"] == nil || r["currency"] == nil {
		t.Error("response missing required fields")
	}
	bal, _ := r["balance"].(float64)
	if bal != 1000 {
		t.Errorf("balance = %.0f, want 1000", bal)
	}
	cur, _ := r["currency"].(string)
	if cur != "EUR" {
		t.Errorf("currency = %s, want EUR", cur)
	}
}

func TestSer_TransferResponseSchema(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("schem-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "transfer")
	if r["transfer_id"] == nil || r["status"] == nil || r["amount"] == nil {
		t.Error("response missing required fields")
	}
}

func TestSer_ContractTransferFields(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("contr-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "contract")
	expected := []string{"transfer_id", "status", "from", "to", "amount"}
	for _, f := range expected {
		if r[f] == nil {
			t.Errorf("field %s missing from response", f)
		}
	}
}

func TestSer_ContractFraudResponse(t *testing.T) {
	r := doJSON("POST", baseURL["fraud"]+"/api/v1/fraud/check", `{"amount":100}`)
	assertStatus(t, r["_status"].(float64), 200, "fraud")
	if _, ok := r["fraud"].(bool); !ok {
		t.Error("fraud field missing or wrong type")
	}
}

func TestSer_NestedJSON(t *testing.T) {
	payload := `{"from_account_id":"1","to_account_id":"2","amount":50.5,"idempotency_key":"nested-test"}`
	var parsed map[string]any
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatal("valid JSON should parse")
	}
	amt, _ := parsed["amount"].(float64)
	if amt != 50.5 {
		t.Errorf("amount = %.1f, want 50.5", amt)
	}
}

func TestSer_FloatPrecision(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	key := fmt.Sprintf("fp-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50.75,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "float transfer")
	amt, _ := r["amount"].(float64)
	if amt != 50.75 {
		t.Errorf("amount = %.2f, want 50.75", amt)
	}
}

func TestSer_LargeInteger(t *testing.T) {
	key := fmt.Sprintf("bigint-%d", time.Now().UnixNano())
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"999999999","to_account_id":"1","amount":1,"idempotency_key":"%s"}`, key), demoToken)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Log("large integer accepted (async)")
	} else if st >= 400 {
		t.Logf("large integer rejected: %d", st)
	}
}

func TestSer_EmptyStringFields(t *testing.T) {
	r := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup",
		`{"username":"","password":""}`)
	assertStatus(t, r["_status"].(float64), 400, "empty fields")
}

func TestSer_WhiteSpaceFields(t *testing.T) {
	r := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup",
		`{"username":"   ","password":"   "}`)
	st := int(r["_status"].(float64))
	if st == 201 {
		t.Error("whitespace fields should not create user")
	}
}

func TestSer_DecimalCurrency(t *testing.T) {
	r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts",
		`{"currency":"USD"}`, demoToken)
	assertStatus(t, r["_status"].(float64), 201, "USD")
	bal, _ := r["balance"].(float64)
	if bal < 999 || bal > 1001 {
		t.Errorf("unexpected balance %.2f", bal)
	}
}

func TestSer_MultipleCurrencies(t *testing.T) {
	for _, cur := range []string{"USD", "EUR", "GBP", "JPY", "BTC"} {
		r := doJSONAuth("POST", baseURL["account"]+"/api/v1/accounts",
			fmt.Sprintf(`{"currency":"%s"}`, cur), demoToken)
		if int(r["_status"].(float64)) != 201 {
			t.Errorf("currency %s failed: %v", cur, r["_status"])
		}
	}
}
