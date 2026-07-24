package svc

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"tickets/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/runtime"
)

type ServiceContext struct {
	svc         *runtime.Service
	ticketTable *db.Table[models.Ticket]
	orderTable  *db.Table[models.Order]
	confirmed   []models.OrderEvent
	mu          sync.RWMutex
	callbacks   [][]byte
	callbackMu  sync.RWMutex
}

func NewServiceContext(s *runtime.Service) *ServiceContext {
	return &ServiceContext{svc: s}
}

func (c *ServiceContext) Pool() *pgxpool.Pool {
	return c.svc.PoolPGTyped("pg-main")
}

func (c *ServiceContext) NATS() events.EventBroker {
	return c.svc.NATS("default")
}

func (c *ServiceContext) SetOrderTable(tbl *db.Table[models.Order]) {
	c.orderTable = tbl
}

func (c *ServiceContext) OrderTable() *db.Table[models.Order] {
	return c.orderTable
}

func (c *ServiceContext) SetTicketTable(tbl *db.Table[models.Ticket]) {
	c.ticketTable = tbl
}

func (c *ServiceContext) TicketTable() *db.Table[models.Ticket] {
	return c.ticketTable
}

func (c *ServiceContext) DecrementStock(ctx context.Context, ticketID int64, qty int) (bool, error) {
	tag, err := c.Pool().Exec(ctx, "UPDATE tickets SET stock = stock - $1 WHERE id = $2 AND stock >= $1", qty, ticketID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (c *ServiceContext) CreateOrder(ctx context.Context, ticketID int64, qty int) (int64, error) {
	order := models.Order{TicketID: ticketID, Quantity: qty, Status: "confirmed"}
	if err := c.orderTable.Create(ctx, &order); err != nil {
		return 0, err
	}
	return order.ID, nil
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

func (c *ServiceContext) OnOrderConfirmed(ctx context.Context, msg []byte) ([]byte, error) {
	var evt models.OrderEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		return nil, err
	}
	time.Sleep(50 * time.Millisecond)
	c.RecordConfirmation(evt)
	return nil, nil
}

func (c *ServiceContext) OnBatchPayment(ctx context.Context, msg []byte) ([]byte, error) {
	var evt models.OrderEvent
	if err := json.Unmarshal(msg, &evt); err != nil {
		return nil, err
	}
	time.Sleep(100 * time.Millisecond)
	return nil, nil
}

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

func (c *ServiceContext) OnDailyReport(ctx context.Context) error {
	log.Println("[cron] daily report generated")
	return nil
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
	_, err := c.ticketTable.Update(ctx, ticketID, runtime.Map{"stock": stock})
	return err
}
