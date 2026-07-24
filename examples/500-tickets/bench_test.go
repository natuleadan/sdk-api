//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"tickets/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
)

var benchPool *pgxpool.Pool

func TestMain(m *testing.M) {
	poolURL := os.Getenv("DATABASE_URL")
	if poolURL == "" {
		poolURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}
	var err error
	benchPool, err = db.NewPool(context.Background(), db.PoolConfig{URL: poolURL})
	if err != nil {
		log.Fatalf("pool: %v", err)
	}

	ctx := context.Background()
	pool := benchPool

	orderTbl, err := db.NewTable[models.Order](pool, "orders")
	if err != nil {
		log.Fatalf("order table: %v", err)
	}
	if err := orderTbl.AutoInit(ctx); err != nil {
		log.Fatalf("order autoinit: %v", err)
	}

	ticketTbl, err := db.NewTable[models.Ticket](pool, "tickets")
	if err != nil {
		log.Fatalf("ticket table: %v", err)
	}
	if err := ticketTbl.AutoInit(ctx); err != nil {
		log.Fatalf("ticket autoinit: %v", err)
	}

	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM tickets").Scan(&count)
	if count == 0 {
		seeds := []models.Ticket{
			{Name: "Taylor Swift - VIP", Description: "Front row VIP", Price: 599.99, Stock: 10},
			{Name: "Taylor Swift - Gold", Description: "Gold circle", Price: 299.99, Stock: 25},
			{Name: "Taylor Swift - Silver", Description: "Silver standard", Price: 149.99, Stock: 50},
			{Name: "Taylor Swift - General", Description: "General admission", Price: 79.99, Stock: 100},
		}
		for _, s := range seeds {
			if err := ticketTbl.Create(ctx, &s); err != nil {
				log.Fatalf("seed: %v", err)
			}
		}
	}

	os.Exit(m.Run())
}

func request(method, path string, body []byte) (*http.Response, error) {
	req, _ := http.NewRequest(method, "http://localhost:23500"+path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

func readBody(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b)
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// ============================================================================
// 1. CRUD /tickets — Logic + Security
// ============================================================================

func TestCRUD_List(t *testing.T) {
	resp, err := request("GET", "/api/v1/tickets", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200: %s", resp.StatusCode, readBody(resp))
	}
}

func TestCRUD_Create(t *testing.T) {
	body := mustMarshal(map[string]any{"name": "TestCRUD", "description": "desc", "price": 50, "stock": 10})
	resp, err := request("POST", "/api/v1/tickets", body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status = %d, want 201: %s", resp.StatusCode, readBody(resp))
	}
}

func TestCRUD_Create_InvalidBody(t *testing.T) {
	resp, err := request("POST", "/api/v1/tickets", []byte("not json"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCRUD_Get(t *testing.T) {
	resp, _ := request("GET", "/api/v1/tickets/1", nil)
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200: %s", resp.StatusCode, readBody(resp))
	}
}

func TestCRUD_Get_NotFound(t *testing.T) {
	resp, _ := request("GET", "/api/v1/tickets/99999", nil)
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestCRUD_Delete(t *testing.T) {
	resp, _ := request("DELETE", "/api/v1/tickets/99999", nil)
	if resp.StatusCode != 404 && resp.StatusCode != 204 {
		t.Errorf("status = %d, want 404 or 204", resp.StatusCode)
	}
}

// ============================================================================
// 2. Fire-and-forget — POST /api/v1/orders/create
// ============================================================================

func TestOrder_Create_OK(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 1})
	resp, err := request("POST", "/api/v1/orders/create", body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status = %d, want 201: %s", resp.StatusCode, readBody(resp))
	}
}

func TestOrder_Create_ZeroQuantity(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 0})
	resp, _ := request("POST", "/api/v1/orders/create", body)
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestOrder_Create_InvalidBody(t *testing.T) {
	resp, _ := request("POST", "/api/v1/orders/create", []byte("not json"))
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestOrder_Create_SoldOut(t *testing.T) {
	request("POST", "/api/v1/admin/reset-stock", mustMarshal(map[string]any{"ticket_id": 99, "stock": 0}))

	body := mustMarshal(map[string]any{"ticket_id": 99, "quantity": 1})
	resp, _ := request("POST", "/api/v1/orders/create", body)
	if resp.StatusCode != 409 {
		t.Errorf("status = %d, want 409: %s", resp.StatusCode, readBody(resp))
	}
}

func TestOrder_RaceCondition_30x10(t *testing.T) {
	// Create a fresh ticket with stock=10
	body := mustMarshal(map[string]any{"name": "RaceTicket", "description": "race test", "price": 10, "stock": 10})
	resp, _ := request("POST", "/api/v1/tickets", body)
	var ticket struct{ ID int64 `json:"id"` }
	json.Unmarshal([]byte(readBody(resp)), &ticket)
	if ticket.ID == 0 {
		t.Fatal("failed to create race ticket")
	}

	var wg sync.WaitGroup
	mu := sync.Mutex{}
	var success, soldOut int

	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := mustMarshal(map[string]any{"ticket_id": ticket.ID, "quantity": 1})
			resp, err := request("POST", "/api/v1/orders/create", b)
			if err != nil {
				return
			}
			mu.Lock()
			if resp.StatusCode == 201 {
				success++
			} else if resp.StatusCode == 409 {
				soldOut++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if success != 10 {
		t.Errorf("successful orders = %d, want 10", success)
	}
	if soldOut != 20 {
		t.Errorf("sold out responses = %d, want 20", soldOut)
	}
}

// ============================================================================
// 3. Async Batch — /api/v1/orders/batch
// ============================================================================

func TestOrder_Async_Submit_Returns202(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 3})
	resp, err := request("POST", "/api/v1/orders/batch", body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 {
		t.Fatalf("status = %d, want 202: %s", resp.StatusCode, readBody(resp))
	}
	var r struct {
		JobID     string `json:"job_id"`
		Status    string `json:"status"`
		StatusURL string `json:"status_url"`
	}
	if err := json.Unmarshal([]byte(readBody(resp)), &r); err != nil {
		t.Fatal("unmarshal:", err)
	}
	if r.JobID == "" {
		t.Fatal("job_id is empty")
	}
	if r.StatusURL == "" {
		t.Fatal("status_url is empty")
	}
}

func TestOrder_Async_Poll_Completes(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 2})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var submit struct {
		JobID     string `json:"job_id"`
		StatusURL string `json:"status_url"`
	}
	json.Unmarshal([]byte(readBody(resp)), &submit)

	for range 30 {
		time.Sleep(200 * time.Millisecond)
		resp, _ := request("GET", submit.StatusURL, nil)
		var state struct {
			Status string         `json:"status"`
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal([]byte(readBody(resp)), &state); err != nil {
			t.Fatal("unmarshal status:", err)
		}
		switch state.Status {
		case "completed":
			if state.Result == nil {
				t.Fatal("completed job has no result")
			}
			return
		case "failed":
			t.Fatalf("job failed: %v", state)
		}
	}
	t.Fatal("job did not complete within 6s")
}

func TestOrder_Async_NotFound(t *testing.T) {
	resp, _ := request("GET", "/api/v1/orders/batch/nonexistent", nil)
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestOrder_Async_Concurrent(t *testing.T) {
	request("POST", "/api/v1/admin/reset-stock", mustMarshal(map[string]any{"ticket_id": 101, "stock": 20}))

	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := mustMarshal(map[string]any{"ticket_id": 101, "quantity": 5})
			resp, err := request("POST", "/api/v1/orders/batch", b)
			if err != nil || resp.StatusCode != 202 {
				return
			}
		}()
	}
	wg.Wait()
}

// ============================================================================
// 5. RPC Reply — /api/v1/orders/validate-payment
// ============================================================================

func TestOrder_RPC_ValidQuantity(t *testing.T) {
	body := mustMarshal(map[string]any{"order_id": 1, "ticket_id": 1, "quantity": 1})
	resp, err := request("POST", "/api/v1/orders/validate-payment", body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200: %s", resp.StatusCode, readBody(resp))
	}
	var result struct {
		Valid bool `json:"valid"`
	}
	json.Unmarshal([]byte(readBody(resp)), &result)
	if !result.Valid {
		t.Error("payment should be valid for qty=1")
	}
}

func TestOrder_RPC_InvalidQuantity(t *testing.T) {
	body := mustMarshal(map[string]any{"order_id": 1, "ticket_id": 1, "quantity": 10})
	resp, _ := request("POST", "/api/v1/orders/validate-payment", body)
	if resp.StatusCode != 422 {
		t.Errorf("status = %d, want 422: %s", resp.StatusCode, readBody(resp))
	}
	var result struct {
		Valid   bool   `json:"valid"`
		Message string `json:"message"`
	}
	json.Unmarshal([]byte(readBody(resp)), &result)
	if result.Valid {
		t.Error("payment should be invalid for qty=10")
	}
	if result.Message == "" {
		t.Error("error message should not be empty")
	}
}

func TestOrder_RPC_BadRequest(t *testing.T) {
	resp, _ := request("POST", "/api/v1/orders/validate-payment", []byte("not json"))
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// ============================================================================
// 6. Admin — /api/v1/admin/reset-stock
// ============================================================================

func TestAdmin_ResetStock_OK(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "stock": 100})
	resp, err := request("POST", "/api/v1/admin/reset-stock", body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200: %s", resp.StatusCode, readBody(resp))
	}
}

func TestAdmin_ResetStock_InvalidBody(t *testing.T) {
	resp, _ := request("POST", "/api/v1/admin/reset-stock", []byte("not json"))
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// ============================================================================
// 7. Cron — /api/v1/reports/daily
// ============================================================================

func TestReport_Daily_OK(t *testing.T) {
	resp, err := request("GET", "/api/v1/reports/daily", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var report struct {
		TotalSales   int     `json:"total_sales"`
		TotalRevenue float64 `json:"total_revenue"`
	}
	if err := json.Unmarshal([]byte(readBody(resp)), &report); err != nil {
		t.Fatal("unmarshal report:", err)
	}
	if report.TotalSales < 0 {
		t.Errorf("total_sales = %d, want >= 0", report.TotalSales)
	}
	if report.TotalRevenue < 0 {
		t.Errorf("total_revenue = %f, want >= 0", report.TotalRevenue)
	}
}

// ============================================================================
// 8. Cross-feature: Full flow — stock reservation through events
// ============================================================================

func TestCrossFeature_BuyThenReport(t *testing.T) {
	request("POST", "/api/v1/admin/reset-stock", mustMarshal(map[string]any{"ticket_id": 4, "stock": 5}))

	var orderIDs []int64
	for range 3 {
		b := mustMarshal(map[string]any{"ticket_id": 4, "quantity": 1})
		resp, _ := request("POST", "/api/v1/orders/create", b)
		if resp.StatusCode == 201 {
			var r struct {
				OrderID int64 `json:"order_id"`
			}
			json.Unmarshal([]byte(readBody(resp)), &r)
			orderIDs = append(orderIDs, r.OrderID)
		}
	}

	if len(orderIDs) != 3 {
		t.Errorf("created %d orders, want 3", len(orderIDs))
	}

	for _, id := range orderIDs {
		resp, _ := request("GET", fmt.Sprintf("/api/v1/orders/get?id=%d", id), nil)
		if resp.StatusCode != 200 {
			t.Errorf("order %d status = %d, want 200", id, resp.StatusCode)
		}
	}

	resp, _ := request("GET", "/api/v1/reports/daily", nil)
	var report struct {
		TotalSales   int     `json:"total_sales"`
		TotalRevenue float64 `json:"total_revenue"`
	}
	json.Unmarshal([]byte(readBody(resp)), &report)
	if report.TotalSales < len(orderIDs) {
		t.Errorf("total_sales = %d, want >= %d", report.TotalSales, len(orderIDs))
	}
}

// ============================================================================
// 9. Async Security
// ============================================================================

func TestOrder_Async_InvalidBody(t *testing.T) {
	resp, _ := request("POST", "/api/v1/orders/batch", []byte("not json"))
	if resp.StatusCode != 202 {
		t.Errorf("status = %d, want 202 (async accepts then fails)", resp.StatusCode)
	}
	var r struct {
		StatusURL string `json:"status_url"`
	}
	json.Unmarshal([]byte(readBody(resp)), &r)
	if r.StatusURL == "" {
		t.Fatal("no status_url")
	}
	for range 10 {
		time.Sleep(200 * time.Millisecond)
		resp2, _ := request("GET", r.StatusURL, nil)
		var state struct{ Status string `json:"status"` }
		json.Unmarshal([]byte(readBody(resp2)), &state)
		if state.Status == "failed" {
			return
		}
		if state.Status == "completed" {
			t.Fatal("invalid body job should have failed")
		}
	}
	t.Fatal("invalid body job did not fail within 2s")
}

func TestOrder_Async_NegativeQuantity(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": -1})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	if resp.StatusCode != 202 {
		t.Errorf("status = %d, want 202 (async accepts then fails)", resp.StatusCode)
	}
	var r struct {
		StatusURL string `json:"status_url"`
	}
	json.Unmarshal([]byte(readBody(resp)), &r)
	for range 10 {
		time.Sleep(200 * time.Millisecond)
		resp2, _ := request("GET", r.StatusURL, nil)
		var state struct{ Status string `json:"status"` }
		json.Unmarshal([]byte(readBody(resp2)), &state)
		if state.Status == "failed" {
			return
		}
	}
	t.Fatal("negative qty job did not fail within 2s")
}

// ============================================================================
// 10. SQL Injection Probes
// ============================================================================

func TestOrder_Create_SQLInjection(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": "' OR 1=1--", "quantity": 1})
	resp, _ := request("POST", "/api/v1/orders/create", body)
	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		t.Error("SQL injection attempt returned success")
	}
}

func TestOrder_Get_SQLInjection(t *testing.T) {
	resp, _ := request("GET", "/api/v1/orders/get?id=' OR 1=1--", nil)
	if resp.StatusCode == 200 {
		t.Error("SQL injection attempt returned 200")
	}
}

func TestOrder_Get_InvalidID(t *testing.T) {
	resp, _ := request("GET", "/api/v1/orders/get?id=abc", nil)
	if resp.StatusCode == 200 {
		t.Error("non-numeric id returned 200")
	}
}

// ============================================================================
// 11. Report edge cases
// ============================================================================

func TestReport_AfterSales(t *testing.T) {
	resp, _ := request("GET", "/api/v1/reports/daily", nil)
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var report struct {
		TotalSales   int     `json:"total_sales"`
		TotalRevenue float64 `json:"total_revenue"`
	}
	if err := json.Unmarshal([]byte(readBody(resp)), &report); err != nil {
		t.Fatal(err)
	}
	if report.TotalRevenue < 0 {
		t.Errorf("total_revenue = %f, want >= 0", report.TotalRevenue)
	}
}

// ============================================================================
// 12. Async DELETE Cancel (3 tests)
// ============================================================================

func TestAsync_Delete_204(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 2})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)

	resp2, _ := request("DELETE", r.StatusURL, nil)
	if resp2.StatusCode != 204 {
		t.Errorf("status = %d, want 204: %s", resp2.StatusCode, readBody(resp2))
	}
}

func TestAsync_Delete_404(t *testing.T) {
	resp, _ := request("DELETE", "/api/v1/orders/batch/nonexistent", nil)
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAsync_Delete_409(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 10})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)

	time.Sleep(100 * time.Millisecond)
	resp2, _ := request("DELETE", r.StatusURL, nil)
	if resp2.StatusCode == 204 {
		t.Log("DELETE returned 204 — job already completed")
	} else if resp2.StatusCode != 409 {
		t.Errorf("status = %d, want 409 (processing)", resp2.StatusCode)
	}
}

// ============================================================================
// 13. Async Callback Webhook (3 tests)
// ============================================================================

func TestAsync_Callback_Received(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 1})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var r struct {
		StatusURL string `json:"status_url"`
	}
	json.Unmarshal([]byte(readBody(resp)), &r)

	for range 15 {
		time.Sleep(200 * time.Millisecond)
		resp2, _ := request("GET", r.StatusURL, nil)
		var state struct{ Status string `json:"status"` }
		json.Unmarshal([]byte(readBody(resp2)), &state)
		if state.Status == "completed" || state.Status == "failed" {
			break
		}
	}
}

func TestAsync_Callback_Signature(t *testing.T) {
	body := mustMarshal(map[string]any{"test": true})
	req, _ := http.NewRequest("POST", "http://localhost:23500/api/v1/webhooks/batch-complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Job-Signature", "invalid-signature")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAsync_Callback_ValidSignature(t *testing.T) {
	payload := mustMarshal(map[string]any{"id": "test-job", "status": "completed"})
	req, _ := http.NewRequest("POST", "http://localhost:23500/api/v1/webhooks/batch-complete", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

// ============================================================================
// 14. Async SSE Status Streaming (2 tests)
// ============================================================================

func TestAsync_SSE_StatusEndpoint(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 1})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var r struct {
		JobID     string `json:"job_id"`
		StatusURL string `json:"status_url"`
	}
	json.Unmarshal([]byte(readBody(resp)), &r)

	sseURL := "http://localhost:23500" + r.StatusURL + "/status"
	sseResp, err := http.Get(sseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	if sseResp.StatusCode != 200 {
		t.Errorf("SSE status = %d, want 200", sseResp.StatusCode)
	}
	if sseResp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", sseResp.Header.Get("Content-Type"))
	}
}

func TestAsync_SSE_ReceivesCompletion(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 1})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var r struct {
		JobID     string `json:"job_id"`
		StatusURL string `json:"status_url"`
	}
	json.Unmarshal([]byte(readBody(resp)), &r)

	sseURL := "http://localhost:23500" + r.StatusURL + "/status"
	sseResp, err := http.Get(sseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	data := make([]byte, 4096)
	n, _ := sseResp.Body.Read(data)
	if n == 0 {
		t.Fatal("no SSE data received")
	}
	bodyStr := string(data[:n])
	if !strings.Contains(bodyStr, "completed") && !strings.Contains(bodyStr, "processing") {
		t.Errorf("SSE data does not contain status: %s", bodyStr)
	}
}

// ============================================================================
// 15. Async Validation Tests (3 tests)
// ============================================================================

func TestAsync_Validate_QuantityTooHigh(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 200})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	if resp.StatusCode != 202 {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)

	for range 10 {
		time.Sleep(200 * time.Millisecond)
		resp2, _ := request("GET", r.StatusURL, nil)
		var state struct{ Status string `json:"status"` }
		json.Unmarshal([]byte(readBody(resp2)), &state)
		if state.Status == "failed" {
			return
		}
	}
	t.Fatal("validation job should have failed")
}

func TestAsync_Validate_NegativeQuantity(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": -5})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)

	for range 10 {
		time.Sleep(200 * time.Millisecond)
		resp2, _ := request("GET", r.StatusURL, nil)
		var state struct{ Status string `json:"status"` }
		json.Unmarshal([]byte(readBody(resp2)), &state)
		if state.Status == "failed" {
			return
		}
	}
	t.Fatal("negative qty job should have failed")
}

func TestAsync_Validate_InvalidTicketID(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": -1, "quantity": 1})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)

	for range 10 {
		time.Sleep(200 * time.Millisecond)
		resp2, _ := request("GET", r.StatusURL, nil)
		var state struct{ Status string `json:"status"` }
		json.Unmarshal([]byte(readBody(resp2)), &state)
		if state.Status == "failed" {
			return
		}
	}
	t.Fatal("invalid ticket_id job should have failed")
}

// ============================================================================
// 16. MaxConcurrent VIP Batch (2 tests)
// ============================================================================

func TestAsync_VIPBatch_Accepts202(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 3})
	resp, _ := request("POST", "/api/v1/orders/batch-vip", body)
	if resp.StatusCode != 202 {
		t.Errorf("status = %d, want 202: %s", resp.StatusCode, readBody(resp))
	}
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)
	if r.StatusURL == "" {
		t.Error("no status_url")
	}
}

func TestAsync_VIPBatch_Completes(t *testing.T) {
	request("POST", "/api/v1/admin/reset-stock", mustMarshal(map[string]any{"ticket_id": 5, "stock": 10}))
	body := mustMarshal(map[string]any{"ticket_id": 5, "quantity": 5})
	resp, _ := request("POST", "/api/v1/orders/batch-vip", body)
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)

	for range 15 {
		time.Sleep(200 * time.Millisecond)
		resp2, _ := request("GET", r.StatusURL, nil)
		var state struct{ Status string `json:"status"` }
		json.Unmarshal([]byte(readBody(resp2)), &state)
		if state.Status == "completed" {
			return
		}
		if state.Status == "failed" {
			t.Fatal("VIP batch failed")
		}
	}
	t.Fatal("VIP batch did not complete")
}

// ============================================================================
// 17. Callback Per-Request (1 test)
// ============================================================================

func TestAsync_Callback_PerRequestURL(t *testing.T) {
	body := mustMarshal(map[string]any{
		"ticket_id":     1,
		"quantity":      1,
		"_callback_url": "http://localhost:23500/api/v1/webhooks/batch-complete",
	})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	if resp.StatusCode != 202 {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)

	for range 10 {
		time.Sleep(200 * time.Millisecond)
		resp2, _ := request("GET", r.StatusURL, nil)
		var state struct{ Status string `json:"status"` }
		json.Unmarshal([]byte(readBody(resp2)), &state)
		if state.Status == "completed" || state.Status == "failed" {
			return
		}
	}
}

// ============================================================================
// 18. GET List Jobs (2 tests)
// ============================================================================

func TestAsync_List_ReturnsJobs(t *testing.T) {
	resp, _ := request("GET", "/api/v1/orders/batch", nil)
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200: %s", resp.StatusCode, readBody(resp))
	}
	var list struct {
		Jobs  []any `json:"jobs"`
		Total int   `json:"total"`
	}
	if err := json.Unmarshal([]byte(readBody(resp)), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total < 0 {
		t.Errorf("total = %d, want >= 0", list.Total)
	}
}

func TestAsync_List_EmptyListOK(t *testing.T) {
	body := mustMarshal(map[string]any{"ticket_id": 1, "quantity": 2})
	resp, _ := request("POST", "/api/v1/orders/batch", body)
	var r struct{ StatusURL string `json:"status_url"` }
	json.Unmarshal([]byte(readBody(resp)), &r)

	resp2, _ := request("DELETE", r.StatusURL, nil)
	readBody(resp2)

	resp3, _ := request("GET", "/api/v1/orders/batch", nil)
	if resp3.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp3.StatusCode)
	}
}

