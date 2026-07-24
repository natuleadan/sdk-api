package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// OutboxRecord represents a pending event in the outbox table.
type OutboxRecord struct {
	ID        int64  `db:"id,primary,auto"`
	Subject   string `db:"subject,required"`
	Payload   string `db:"payload,required"`
	Status    string `db:"status,required,default='pending'"`
	CreatedAt string `db:"created_at,default=now()"`
}

// OutboxRelay polls the outbox table and publishes pending events to NATS.
// It runs in a background goroutine and is opt-in via event_publish.outbox: true.
type OutboxRelay struct {
	pool   *pgxpool.Pool
	broker events.EventBroker
	table  *db.Table[OutboxRecord]
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewOutboxRelay creates a relay that polls pending events and publishes them.
func NewOutboxRelay(pool *pgxpool.Pool, broker events.EventBroker) (*OutboxRelay, error) {
	ctx, cancel := context.WithCancel(context.Background())
	tbl, err := db.NewTable[OutboxRecord](pool, "_outbox")
	if err != nil {
		cancel()
		return nil, err
	}
	if err := tbl.AutoInit(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("outbox autoinit: %w", err)
	}
	return &OutboxRelay{
		pool:   pool,
		broker: broker,
		table:  tbl,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start begins polling in a background goroutine.
func (r *OutboxRelay) Start() {
	r.wg.Go(func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				r.relayOnce()
			}
		}
	})
	logx.Info("outbox relay started")
}

func (r *OutboxRelay) relayOnce() {
	records, err := r.table.QueryWhere(r.ctx, Map{"status": "pending"}, "id", 100, 0)
	if err != nil {
		return
	}
	for _, rec := range records {
		if err := r.broker.Publish(r.ctx, rec.Subject, []byte(rec.Payload)); err != nil {
			logx.Errorf("outbox: publish %s: %v", rec.Subject, err)
			continue
		}
		if _, err := r.pool.Exec(r.ctx, `UPDATE _outbox SET status = 'published' WHERE id = $1`, rec.ID); err != nil {
			logx.Errorf("outbox: mark published %d: %v", rec.ID, err)
		}
	}
}

// Stop stops the relay and waits for in-flight publishes.
func (r *OutboxRelay) Stop() {
	r.cancel()
	r.wg.Wait()
	logx.Info("outbox relay stopped")
}

// InsertOutbox adds an event to the outbox table for transactional publishing.
// Called from within a DB transaction alongside the business logic write.
func InsertOutbox(ctx context.Context, pool *pgxpool.Pool, subject string, payload []byte) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO _outbox (subject, payload, status) VALUES ($1, $2, 'pending')`,
		subject, string(payload))
	return err
}

// InsertOutboxJSON is a convenience wrapper for JSON payloads.
func InsertOutboxJSON(ctx context.Context, pool *pgxpool.Pool, subject string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return InsertOutbox(ctx, pool, subject, b)
}
