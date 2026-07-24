package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
)

func getProcessingTimeout(entry *EntryDef) time.Duration {
	if entry.AsyncStore == nil || entry.AsyncStore.Reassign == nil {
		return 5 * time.Minute
	}
	timeout := parseDurationDef(entry.AsyncStore.Reassign.ProcessingTimeout)
	if timeout <= 0 {
		return 5 * time.Minute
	}
	return timeout
}

// resolveAsyncStore creates a JobStore based on the entry's async_store config.
func resolveAsyncStore(entry *EntryDef, pools map[string]any, kvConns map[string]*redis.Redis, brokers map[string]events.EventBroker) (JobStore, error) {
	timeout := getProcessingTimeout(entry)

	if entry.AsyncStore == nil || entry.AsyncStore.Driver == "" || entry.AsyncStore.Driver == "memory" {
		store := newMemoryJobStore()
		store.processingTimeout = timeout
		return store, nil
	}
	switch entry.AsyncStore.Driver {
	case "postgres":
		return resolvePGStore(entry, pools, timeout)
	case "redis":
		return resolveRedisStore(entry, kvConns, timeout)
	case "nats_kv":
		return resolveNATSKVStore(entry, brokers, timeout)
	default:
		return nil, fmt.Errorf("async_store: unknown driver %q", entry.AsyncStore.Driver)
	}
}

func resolvePGStore(entry *EntryDef, pools map[string]any, timeout time.Duration) (JobStore, error) {
	if entry.AsyncStore.DB == "" {
		return nil, fmt.Errorf("async_store: db is required for driver postgres")
	}
	pool := PoolPG(pools, entry.AsyncStore.DB)
	if pool == nil {
		return nil, fmt.Errorf("async_store: pool %q not found", entry.AsyncStore.DB)
	}
	table := entry.AsyncStore.Table
	if table == "" {
		table = "async_jobs"
	}
	store := newPGJobStore(pool, table)
	store.processingTimeout = timeout
	if err := store.ensureTable(context.Background()); err != nil {
		return nil, fmt.Errorf("async_store: create pg table: %w", err)
	}
	return store, nil
}

func resolveRedisStore(entry *EntryDef, kvConns map[string]*redis.Redis, timeout time.Duration) (JobStore, error) {
	if entry.AsyncStore.KV == "" {
		return nil, fmt.Errorf("async_store: kv is required for driver redis")
	}
	client, ok := kvConns[entry.AsyncStore.KV]
	if !ok || client == nil {
		return nil, fmt.Errorf("async_store: kv store %q not found", entry.AsyncStore.KV)
	}
	store := newRedisJobStore(client, "job:")
	store.processingTimeout = timeout
	return store, nil
}

func resolveNATSKVStore(entry *EntryDef, brokers map[string]events.EventBroker, timeout time.Duration) (JobStore, error) {
	if entry.AsyncStore.Stream == "" {
		return nil, fmt.Errorf("async_store: stream is required for driver nats_kv")
	}
	broker, ok := brokers[entry.AsyncStore.Stream]
	if !ok || broker == nil {
		return nil, fmt.Errorf("async_store: stream %q not found", entry.AsyncStore.Stream)
	}
	conn, ok := broker.(*events.Conn)
	if !ok {
		return nil, fmt.Errorf("async_store: stream %q is not a NATS connection", entry.AsyncStore.Stream)
	}
	bucket := entry.AsyncStore.Bucket
	if bucket == "" {
		bucket = "async-jobs"
	}
	if _, err := conn.EnsureKeyValue(events.KVConfig{Bucket: bucket}); err != nil {
		return nil, fmt.Errorf("async_store: ensure kv bucket: %w", err)
	}
	store := newNATSKVJobStore(conn, bucket)
	store.processingTimeout = timeout
	return store, nil
}
