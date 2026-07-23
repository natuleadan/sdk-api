//go:build integration

package runtime

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
)

func getTestPGPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close())
	return pool
}

func getTestRedis(t *testing.T) *redis.Redis {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	r, err := redis.NewRedis(redis.RedisConf{Addr: addr}, redis.WithPass(""))
	if err != nil {
		t.Fatalf("redis.NewRedis: %v", err)
	}
	return r
}

func getTestNATSConn(t *testing.T) *events.Conn {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}
	conn, err := events.NewConn(url, events.WithName("test"))
	if err != nil {
		t.Fatalf("events.NewConn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestPGJobStore_CRUD(t *testing.T) {
	pool := getTestPGPool(t)
	store := newPGJobStore(pool, "test_async_jobs")
	if err := store.ensureTable(context.Background()); err != nil {
		t.Fatalf("ensureTable: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DROP TABLE IF EXISTS test_async_jobs")
	})

	js := store.Create("pg-test-1")
	if js.ID != "pg-test-1" {
		t.Errorf("id = %q, want pg-test-1", js.ID)
	}

	got, ok := store.Get("pg-test-1")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.Status != JobPending {
		t.Errorf("status = %q, want pending", got.Status)
	}

	store.Update("pg-test-1", JobCompleted, map[string]any{"url": "https://example.com"}, "")
	got, ok = store.Get("pg-test-1")
	if !ok {
		t.Fatal("Get after update returned false")
	}
	if got.Status != JobCompleted {
		t.Errorf("status = %q, want completed", got.Status)
	}
	result, ok := got.Result.(map[string]any)
	if !ok || result["url"] != "https://example.com" {
		t.Errorf("result = %v, want {url: https://example.com}", got.Result)
	}

	store.Delete("pg-test-1")
	if _, ok := store.Get("pg-test-1"); ok {
		t.Error("Get returned true after Delete")
	}
}

func TestRedisJobStore_CRUD(t *testing.T) {
	client := getTestRedis(t)
	store := newRedisJobStore(client, "test:")
	t.Cleanup(func() {
		store.Delete("redis-test-1")
	})

	js := store.Create("redis-test-1")
	if js.ID != "redis-test-1" {
		t.Errorf("id = %q, want redis-test-1", js.ID)
	}

	got, ok := store.Get("redis-test-1")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.Status != JobPending {
		t.Errorf("status = %q, want pending", got.Status)
	}

	store.Update("redis-test-1", JobCompleted, "done", "")
	got, ok = store.Get("redis-test-1")
	if !ok {
		t.Fatal("Get after update returned false")
	}
	if got.Status != JobCompleted {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.Result != "done" {
		t.Errorf("result = %v, want done", got.Result)
	}

	store.Delete("redis-test-1")
	if _, ok := store.Get("redis-test-1"); ok {
		t.Error("Get returned true after Delete")
	}
}

func TestNATSKVJobStore_CRUD(t *testing.T) {
	conn := getTestNATSConn(t)
	bucket := "test-async-jobs"
	if _, err := conn.EnsureKeyValue(events.KVConfig{Bucket: bucket}); err != nil {
		t.Fatalf("EnsureKeyValue: %v", err)
	}
	store := newNATSKVJobStore(conn, bucket)
	t.Cleanup(func() {
		store.Delete("nats-test-1")
	})

	js := store.Create("nats-test-1")
	if js.ID != "nats-test-1" {
		t.Errorf("id = %q, want nats-test-1", js.ID)
	}

	got, ok := store.Get("nats-test-1")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.Status != JobPending {
		t.Errorf("status = %q, want pending", got.Status)
	}

	store.Update("nats-test-1", JobCompleted, "nats-done", "")
	got, ok = store.Get("nats-test-1")
	if !ok {
		t.Fatal("Get after update returned false")
	}
	if got.Status != JobCompleted {
		t.Errorf("status = %q, want completed", got.Status)
	}

	store.Delete("nats-test-1")
	if _, ok := store.Get("nats-test-1"); ok {
		t.Error("Get returned true after Delete")
	}
}
