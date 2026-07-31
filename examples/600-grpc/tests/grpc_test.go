//go:build integration

package tests

import (
	"fmt"
	"testing"
)

func TestGRPC_AuthSvcDeductCredit(t *testing.T) {
	cr := doJSONAuth("POST", baseURL["auth"]+"/api/v1/credits/deduct",
		`{"amount":1}`, demoToken)
	t.Logf("deduct response: status=%v body=%v", cr["_status"], cr)
	assertStatus(t, cr["_status"].(float64), 200, "deduct credits")
	if cr["deducted"] == nil {
		t.Fatal("deduct credits via auth-svc: expected deducted")
	}
}

func TestGRPC_UrlSvcCallsAuthSvc(t *testing.T) {
	cr := doJSONAuth("POST", baseURL["url"]+"/api/v1/links",
		`{"target_url":"https://example.com/grpc-test"}`, demoToken)
	t.Logf("create link response: status=%v body=%v", cr["_status"], cr)
	assertStatus(t, cr["_status"].(float64), 201, "create link")
	if cr["short_code"] == "" {
		t.Fatal("create link via url-svc: expected short_code")
	}
}

func TestGRPC_TicketSvcCallsAuthSvc(t *testing.T) {
	cr := doJSONAuth("POST", baseURL["ticket"]+"/api/v1/orders/buy",
		`{"ticket_id":1,"quantity":1}`, demoToken)
	t.Logf("ticket response: status=%v body=%v", cr["_status"], cr)
	assertStatus(t, cr["_status"].(float64), 201, "buy ticket")
	if cr["status"] != "confirmed" {
		t.Fatalf("buy ticket via ticket-svc: expected confirmed, got %v", cr["status"])
	}
}

func TestGRPC_TransferSvcCallsAccountSvc(t *testing.T) {
	fromID := createAccount(t)
	toID := createAccount(t)

	body := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":50,"currency":"USD"}`,
		fromID, toID)
	cr := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", body, demoToken)
	t.Logf("transfer response: status=%v body=%v", cr["_status"], cr)
	assertStatus(t, cr["_status"].(float64), 201, "initiate transfer")
	if cr["status"] != "initiated" {
		t.Fatalf("initiate transfer: expected initiated, got %v", cr["status"])
	}
	if _, ok := cr["transfer_id"]; !ok {
		t.Fatal("initiate transfer: expected transfer_id")
	}
}

func TestGRPC_TransferSvcInsufficientBalance(t *testing.T) {
	fromID := createAccount(t)
	toID := createAccount(t)

	body := fmt.Sprintf(`{"from_account_id":"%s","to_account_id":"%s","amount":999999,"currency":"USD"}`,
		fromID, toID)
	cr := doJSONAuth("POST", baseURL["transfer"]+"/api/v1/transfers", body, demoToken)
	t.Logf("insufficient response: status=%v body=%v", cr["_status"], cr)
	assertStatus(t, cr["_status"].(float64), 402, "insufficient balance")
}
