package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// pgJobStore persists job state in a PostgreSQL table.
type pgJobStore struct {
	pool  *pgxpool.Pool
	table string
}

func newPGJobStore(pool *pgxpool.Pool, table string) *pgJobStore {
	return &pgJobStore{pool: pool, table: table}
}

func (s *pgJobStore) ensureTable(ctx context.Context) error {
	q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id TEXT PRIMARY KEY,
		status TEXT NOT NULL DEFAULT 'pending',
		result JSONB,
		error TEXT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		retry_count INT NOT NULL DEFAULT 0,
		max_retries INT NOT NULL DEFAULT 0,
		processing_deadline TIMESTAMPTZ
	)`, s.table)
	_, err := s.pool.Exec(ctx, q)
	return err
}

func (s *pgJobStore) Create(id string) *JobState {
	now := time.Now()
	js := &JobState{ID: id, Status: JobPending, CreatedAt: now, UpdatedAt: now}
	q := fmt.Sprintf("INSERT INTO %s (id, status, created_at, updated_at) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING", s.table)
	if _, err := s.pool.Exec(context.Background(), q, id, string(JobPending), now, now); err != nil {
		logx.Errorf("pgJobStore.Create: %v", err)
	}
	return js
}

func (s *pgJobStore) Get(id string) (*JobState, bool) {
	q := fmt.Sprintf("SELECT id, status, result, error, created_at, updated_at, retry_count, max_retries, processing_deadline FROM %s WHERE id = $1", s.table)
	row := s.pool.QueryRow(context.Background(), q, id)
	var js JobState
	var resultBytes []byte
	if err := row.Scan(&js.ID, (*string)(&js.Status), &resultBytes, &js.Error, &js.CreatedAt, &js.UpdatedAt, &js.RetryCount, &js.MaxRetries, &js.ProcessingDeadline); err != nil {
		return nil, false
	}
	if len(resultBytes) > 0 {
		if err := json.Unmarshal(resultBytes, &js.Result); err != nil {
			logx.Errorf("pgJobStore.Get: unmarshal result: %v", err)
		}
	}
	return &js, true
}

func (s *pgJobStore) Update(id string, status JobStatus, result any, errMsg string) {
	dl := any(nil)
	if status == JobProcessing {
		dl = time.Now().Add(5 * time.Minute)
	}
	q := fmt.Sprintf(`UPDATE %s SET status = $1, result = $2, error = $3, updated_at = NOW(),
		processing_deadline = $4 WHERE id = $5`, s.table)
	var resultBytes []byte
	if result != nil {
		resultBytes, _ = json.Marshal(result)
	}
	if _, err := s.pool.Exec(context.Background(), q, string(status), resultBytes, errMsg, dl, id); err != nil {
		logx.Errorf("pgJobStore.Update: %v", err)
	}
}

func (s *pgJobStore) Delete(id string) {
	q := fmt.Sprintf("DELETE FROM %s WHERE id = $1", s.table)
	if _, err := s.pool.Exec(context.Background(), q, id); err != nil {
		logx.Errorf("pgJobStore.Delete: %v", err)
	}
}

func (s *pgJobStore) ReapStale(_ context.Context, timeout time.Duration, maxRetries int) (int, error) {
	q := fmt.Sprintf(`WITH expired AS (
		SELECT id FROM %s
		WHERE status = 'processing'
		AND processing_deadline < NOW()
		AND retry_count < $1
		LIMIT 100
	)
	UPDATE %s SET
		status = 'pending',
		retry_count = retry_count + 1,
		processing_deadline = NULL,
		updated_at = NOW()
	FROM expired
	WHERE %s.id = expired.id`, s.table, s.table, s.table)

	tag, err := s.pool.Exec(context.Background(), q, maxRetries)
	if err != nil {
		return 0, err
	}
	reaped := int(tag.RowsAffected())

	q2 := fmt.Sprintf(`WITH expired AS (
		SELECT id FROM %s
		WHERE status = 'processing'
		AND processing_deadline < NOW()
		AND retry_count >= $1
		LIMIT 100
	)
	UPDATE %s SET
		status = 'failed',
		error = 'max retries exceeded',
		processing_deadline = NULL,
		updated_at = NOW()
	FROM expired
	WHERE %s.id = expired.id`, s.table, s.table, s.table)

	tag2, err := s.pool.Exec(context.Background(), q2, maxRetries)
	if err != nil {
		return reaped, err
	}
	return reaped + int(tag2.RowsAffected()), nil
}

func (s *pgJobStore) Cleanup(_ context.Context, ttl time.Duration) (int, error) {
	q := fmt.Sprintf("DELETE FROM %s WHERE status IN ('completed', 'failed') AND updated_at < NOW() - $1::interval", s.table)
	tag, err := s.pool.Exec(context.Background(), q, ttl.String())
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
