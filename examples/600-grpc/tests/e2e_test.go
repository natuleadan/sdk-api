//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestE2E_FullTransferFlow(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)

	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	assertStatus(t, b1["_status"].(float64), 200, "balance")
	v1, _ := b1["balance"].(float64)
	if v1 != 1000 {
		t.Fatalf("balance = %.0f, want 1000", v1)
	}

	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":200,"idempotency_key":"e2e-%d"}`, ac1, ac2, time.Now().UnixNano()), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "transfer")
	time.Sleep(2 * time.Second)

	ba := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	bb := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac2+"/balance", "", demoToken)
	fa, _ := ba["balance"].(float64)
	fb, _ := bb["balance"].(float64)
	if fa != 800 {
		t.Errorf("sender = %.0f, want 800", fa)
	}
	if fb != 1200 {
		t.Errorf("receiver = %.0f, want 1200", fb)
	}
}

func TestE2E_MultiTransfer(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	base := fmt.Sprintf("e2em-%d", time.Now().UnixNano())
	for i := range 5 {
		key := fmt.Sprintf("%s-%d", base, i)
		r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
			fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"%s"}`, ac1, ac2, key), demoToken)
		assertStatus(t, r["_status"].(float64), 201, fmt.Sprintf("m%d", i))
	}
}

func TestE2E_FraudSmall(t *testing.T) {
	r := doJSON("POST", baseURL["fraud"]+"/api/v1/fraud/check", `{"amount":500}`)
	assertStatus(t, r["_status"].(float64), 200, "fraud")
	if r["fraud"] == true {
		t.Error("500 flagged as fraud")
	}
}

func TestE2E_FraudLarge(t *testing.T) {
	r := doJSON("POST", baseURL["fraud"]+"/api/v1/fraud/check", `{"amount":50000}`)
	assertStatus(t, r["_status"].(float64), 200, "fraud large")
	if r["fraud"] != true {
		t.Error("50000 not flagged")
	}
}

func TestE2E_AllHealthz(t *testing.T) {
	for name, url := range baseURL {
		// Some services may not have /healthz; try GET on root
		paths := []string{"/healthz", "/", "/api/v1/"}
		ok := false
		for _, p := range paths {
			resp, err := http.Get(url + p)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode < 500 {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("%s: no responding endpoint", name)
		}
	}
}

func TestE2E_AuthFlow(t *testing.T) {
	u := fmt.Sprintf("e2e-u-%d", time.Now().UnixNano())
	r1 := doJSON("POST", baseURL["auth"]+"/api/v1/auth/signup",
		fmt.Sprintf(`{"username":"%s","password":"Test1234"}`, u))
	assertStatus(t, r1["_status"].(float64), 201, "signup")
	r2 := doJSON("POST", baseURL["auth"]+"/api/v1/auth/login",
		fmt.Sprintf(`{"username":"%s","password":"Test1234"}`, u))
	assertStatus(t, r2["_status"].(float64), 200, "login")
	if r2["token"] == nil {
		t.Error("no token")
	}
}

func TestE2E_Credits(t *testing.T) {
	r := doJSONAuth("POST", baseURL["auth"]+"/api/v1/credits/buy", `{"amount":10}`, demoToken)
	assertStatus(t, r["_status"].(float64), 200, "buy credits")
}

func TestE2E_CreateURL(t *testing.T) {
	r := doJSONAuth("POST", baseURL["url"]+"/api/v1/links",
		`{"target_url":"https://example.com"}`, demoToken)
	assertStatus(t, r["_status"].(float64), 201, "url")
}

func TestE2E_BuyTicket(t *testing.T) {
	r := doJSONAuth("POST", baseURL["ticket"]+"/api/v1/orders/buy",
		`{"ticket_id":1,"quantity":1}`, demoToken)
	assertStatus(t, r["_status"].(float64), 201, "ticket")
}

func TestE2E_UploadFile(t *testing.T) {
	r := doJSONAuth("POST", baseURL["file"]+"/api/v1/files/upload",
		`{"filename":"test.txt"}`, demoToken)
	assertStatus(t, r["_status"].(float64), 201, "file")
}

func TestE2E_TransferInsufficient(t *testing.T) {
	ac1, ac2 := createAccount(t), createAccount(t)
	r := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":99999,"idempotency_key":"e2e-if-%d"}`, ac1, ac2, time.Now().UnixNano()), demoToken)
	assertStatus(t, r["_status"].(float64), 201, "initiated")
	time.Sleep(2 * time.Second)
	b1 := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac1+"/balance", "", demoToken)
	v1, _ := b1["balance"].(float64)
	if v1 < 0 {
		t.Errorf("insufficient: balance=%.0f", v1)
	}
}

func TestE2E_Receipt(t *testing.T) {
	r := doJSONAuth("POST", baseURL["receipt"]+"/api/v1/receipts",
		`{"transfer_id":"e2e-r","from_account":"1","to_account":"2","amount":200}`, demoToken)
	assertStatus(t, r["_status"].(float64), 201, "receipt")
}

func TestE2E_AllEndpoints(t *testing.T) {
	cases := []struct {
		svc, method, path, body string
		auth                    bool
		min, max                int
	}{
		{"fraud", "POST", "/api/v1/fraud/check", `{"amount":100}`, false, 200, 200},
		{"auth", "POST", "/api/v1/auth/login", `{"username":"demo","password":"demo"}`, false, 200, 200},
		{"account", "POST", "/api/v1/accounts", `{"currency":"USD"}`, true, 201, 201},
		{"transfer", "POST", "/api/v1/transfers", `{}`, true, 400, 400},
	}
	for _, c := range cases {
		var r map[string]any
		if c.auth {
			r = doJSONAuth(c.method, baseURL[c.svc]+c.path, c.body, demoToken)
		} else {
			r = doJSON(c.method, baseURL[c.svc]+c.path, c.body)
		}
		st := int(r["_status"].(float64))
		if st < c.min || st > c.max {
			t.Errorf("%s %s: status %d, want %d-%d", c.svc, c.path, st, c.min, c.max)
		}
	}
}
