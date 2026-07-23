package runtime

import (
	"context"
	"fmt"

	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
)

// resolveAsyncStore creates a JobStore based on the entry's async_store config.
func resolveAsyncStore(entry *EntryDef, pools map[string]any, kvConns map[string]*redis.Redis, brokers map[string]events.EventBroker) (JobStore, error) {
	if entry.AsyncStore == nil || entry.AsyncStore.Driver == "" || entry.AsyncStore.Driver == "memory" {
		return newMemoryJobStore(), nil
	}
	switch entry.AsyncStore.Driver {
	case "postgres":
		return resolvePGStore(entry, pools)
	case "redis":
		return resolveRedisStore(entry, kvConns)
	case "nats_kv":
		return resolveNATSKVStore(entry, brokers)
	default:
		return nil, fmt.Errorf("async_store: unknown driver %q", entry.AsyncStore.Driver)
	}
}

func resolvePGStore(entry *EntryDef, pools map[string]any) (JobStore, error) {
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
	if err := store.ensureTable(context.Background()); err != nil {
		return nil, fmt.Errorf("async_store: create pg table: %w", err)
	}
	return store, nil
}

func resolveRedisStore(entry *EntryDef, kvConns map[string]*redis.Redis) (JobStore, error) {
	if entry.AsyncStore.KV == "" {
		return nil, fmt.Errorf("async_store: kv is required for driver redis")
	}
	client, ok := kvConns[entry.AsyncStore.KV]
	if !ok || client == nil {
		return nil, fmt.Errorf("async_store: kv store %q not found", entry.AsyncStore.KV)
	}
	return newRedisJobStore(client, "job:"), nil
}

func resolveNATSKVStore(entry *EntryDef, brokers map[string]events.EventBroker) (JobStore, error) {
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
	return newNATSKVJobStore(conn, bucket), nil
}
