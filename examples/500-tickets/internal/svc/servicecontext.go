package svc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"tickets/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/runtime"
)

func hmacSign(secret string, data []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

type ServiceContext struct {
	svc           *runtime.Service
	pool          *pgxpool.Pool
	confirmed     []models.OrderEvent
	mu            sync.RWMutex
	callbacks     [][]byte
	callbackMu    sync.RWMutex
	callbackKey   string
}

func NewServiceContext(s *runtime.Service, pool *pgxpool.Pool) *ServiceContext {
	return &ServiceContext{svc: s, pool: pool}
}

func (c *ServiceContext) Pool() *pgxpool.Pool {
	return c.pool
}

func (c *ServiceContext) NATS() events.EventBroker {
	return c.svc.NATS("default")
}

func (c *ServiceContext) EnsureTables(ctx context.Context) error {
	p := c.Pool()
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS tickets (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			price NUMERIC(10,2) NOT NULL DEFAULT 0,
			stock INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS orders (
			id BIGSERIAL PRIMARY KEY,
			ticket_id BIGINT NOT NULL REFERENCES tickets(id),
			quantity INT NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}
	for _, d := range ddl {
		if _, err := p.Exec(ctx, d); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}
	return nil
}

func (c *ServiceContext) SeedData(ctx context.Context) error {
	p := c.Pool()
	var count int
	if err := p.QueryRow(ctx, "SELECT COUNT(*) FROM tickets").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	seeds := []models.Ticket{
		{Name: "Taylor Swift - VIP", Description: "Front row VIP", Price: 599.99, Stock: 10},
		{Name: "Taylor Swift - Gold", Description: "Gold circle", Price: 299.99, Stock: 25},
		{Name: "Taylor Swift - Silver", Description: "Silver standard", Price: 149.99, Stock: 50},
		{Name: "Taylor Swift - General", Description: "General admission", Price: 79.99, Stock: 100},
	}
	for _, s := range seeds {
		if _, err := p.Exec(ctx, "INSERT INTO tickets (name, description, price, stock) VALUES ($1, $2, $3, $4)", s.Name, s.Description, s.Price, s.Stock); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
	}
	log.Printf("seeded %d tickets", len(seeds))
	return nil
}

func (c *ServiceContext) DecrementStock(ctx context.Context, ticketID int64, qty int) (bool, error) {
	tag, err := c.Pool().Exec(ctx, "UPDATE tickets SET stock = stock - $1 WHERE id = $2 AND stock >= $1", qty, ticketID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (c *ServiceContext) CreateOrder(ctx context.Context, ticketID int64, qty int) (int64, error) {
	var id int64
	err := c.Pool().QueryRow(ctx, "INSERT INTO orders (ticket_id, quantity, status) VALUES ($1, $2, 'confirmed') RETURNING id", ticketID, qty).Scan(&id)
	return id, err
}

func (c *ServiceContext) PublishEvent(ctx context.Context, evt models.OrderEvent) error {
	broker := c.svc.NATS("default")
	if broker == nil {
		return nil
	}
	return broker.PublishJSON(ctx, "orders."+evt.Status, evt)
}

func (c *ServiceContext) RecordConfirmation(evt models.OrderEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.confirmed = append(c.confirmed, evt)
}

func (c *ServiceContext) ConfirmedCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.confirmed)
}

// Exit worker: order confirmation (fire-and-forget)
func (c *ServiceContext) OnOrderConfirmed(ctx context.Context, msg []byte) ([]byte, error) {
	var evt models.OrderEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		return nil, err
	}
	time.Sleep(50 * time.Millisecond)
	c.RecordConfirmation(evt)
	return nil, nil
}

// Exit worker: batch payment processing (fire-and-forget)
func (c *ServiceContext) OnBatchPayment(ctx context.Context, msg []byte) ([]byte, error) {
	var evt models.OrderEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		return nil, err
	}
	time.Sleep(100 * time.Millisecond)
	return nil, nil
}

// Exit worker: payment validation (RPC reply)
func (c *ServiceContext) OnValidatePayment(ctx context.Context, msg []byte) ([]byte, error) {
	var evt struct {
		OrderID  int64 `json:"order_id"`
		TicketID int64 `json:"ticket_id"`
		Quantity int   `json:"quantity"`
	}
	if err := json.Unmarshal(msg, &evt); err != nil {
		return nil, err
	}
	time.Sleep(30 * time.Millisecond)
	valid := evt.Quantity <= 5
	resp := map[string]any{"valid": valid}
	if !valid {
		resp["message"] = "quantity exceeds max per order (5)"
	}
	return json.Marshal(resp)
}

// Cron handler: daily report
func (c *ServiceContext) OnDailyReport(ctx context.Context) error {
	log.Println("[cron] daily report generated")
	return nil
}

func (c *ServiceContext) ExpectedSignature(data []byte) string {
	return hmacSign("test-callback-secret", data)
}

func (c *ServiceContext) RecordCallback(data []byte) {
	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()
	c.callbacks = append(c.callbacks, data)
}

func (c *ServiceContext) CallbackCount() int {
	c.callbackMu.RLock()
	defer c.callbackMu.RUnlock()
	return len(c.callbacks)
}

func (c *ServiceContext) ResetStock(ctx context.Context, ticketID int64, stock int) error {
	_, err := c.Pool().Exec(ctx, "UPDATE tickets SET stock = $1 WHERE id = $2", stock, ticketID)
	return err
}
