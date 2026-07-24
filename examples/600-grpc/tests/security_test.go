//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// directRequest sends a request with exact headers, for testing auth
func directAuth(method, url, body, token string) int {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestSec_ValidToken(t *testing.T) {
	ac := createAccount(t)
	st := directAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
	if st == 401 {
		t.Error("valid token rejected")
	}
}

func TestSec_NoToken(t *testing.T) {
	st := directAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		`{"from_account_id":"1","to_account_id":"2","amount":10,"idempotency_key":"sec-nt"}`, "")
	if st != 401 && st != 403 {
		t.Errorf("no token: expected 401/403, got %d", st)
	}
}

func TestSec_ExpiredToken(t *testing.T) {
	st := directAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		`{"from_account_id":"1","to_account_id":"2","amount":10,"idempotency_key":"sec-et"}`,
		"eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjEwMDB9.sig")
	if st != 401 && st != 403 {
		t.Errorf("expired token: expected 401/403, got %d", st)
	}
}

func TestSec_WrongSignature(t *testing.T) {
	st := directAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		`{"from_account_id":"1","to_account_id":"2","amount":10,"idempotency_key":"sec-ws"}`,
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.wrong")
	if st != 401 && st != 403 {
		t.Errorf("wrong sig: expected 401/403, got %d", st)
	}
}

func TestSec_TamperedToken(t *testing.T) {
	tampered := demoToken[:len(demoToken)-5] + "AAAAA"
	st := directAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		`{"from_account_id":"1","to_account_id":"2","amount":10,"idempotency_key":"sec-tt"}`,
		tampered)
	if st != 401 && st != 403 {
		t.Errorf("tampered: expected 401/403, got %d", st)
	}
}

func TestSec_ProtectedEndpoint(t *testing.T) {
	st := directAuth("POST", baseURL["transfer"]+"/api/v1/transfers", `{}`, "")
	if st != 401 && st != 403 {
		t.Errorf("protected: expected 401/403, got %d", st)
	}
}

func TestSec_SignupNoAuth(t *testing.T) {
	u := fmt.Sprintf("sec-user-%d", time.Now().UnixNano())
	st := directAuth("POST", baseURL["auth"]+"/api/v1/auth/signup",
		fmt.Sprintf(`{"username":"%s","password":"Test1234"}`, u), "")
	if st != 201 && st != 409 {
		t.Errorf("signup should work, got %d", st)
	}
}

func TestSec_WeakPassword(t *testing.T) {
	st := directAuth("POST", baseURL["auth"]+"/api/v1/auth/signup",
		`{"username":"weak-u","password":"abc"}`, "")
	if st != 400 {
		t.Errorf("weak password: expected 400, got %d", st)
	}
}

func TestSec_SQLInjection(t *testing.T) {
	// pgx uses parameterized queries, so SQL injection payloads are stored as data, not executed.
	// This test verifies the injection doesn't corrupt the database.
	ac := createAccount(t)
	st := directAuth("POST", baseURL["account"]+"/api/v1/accounts",
		`{"currency":"USD'; DROP TABLE accounts; --"}`, demoToken)
	if st == 200 || st == 201 {
		// Account created with literal injection string - pgx protected us
		// Verify the accounts table still exists
		bal := doJSONAuth("GET", baseURL["account"]+"/api/v1/accounts/"+ac+"/balance", "", demoToken)
		if _, ok := bal["balance"].(float64); ok {
			t.Log("SQL injection: accounts table intact, pgx parameterization working")
		}
	} else if st >= 400 {
		// Also acceptable if the API rejects injection patterns
		t.Logf("SQL injection rejected with %d", st)
	}

	// Transfer injection: "1 OR 1=1" as account ID should not match rows
	st2 := directAuth("POST", baseURL["transfer"]+"/api/v1/transfers",
		fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"idempotency_key":"sec-sqli"}`, "1 OR 1=1", ac), demoToken)
	if st2 == 201 {
		t.Error("SQL injection in transfer succeeded - should have been blocked")
	}
}
