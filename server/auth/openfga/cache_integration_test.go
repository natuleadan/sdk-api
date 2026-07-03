package openfga

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/natuleadan/sdk-api/events"
)

func skipIfNoNATS(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", "localhost:14222")
	if err != nil {
		t.Skipf("NATS not available at localhost:14222: %v", err)
	}
	conn.Close()
}

func TestIntegration_CachedClient_WithNATSKV(t *testing.T) {
	skipIfNoOpenFGA(t)
	skipIfNoNATS(t)

	ctx := context.Background()
	storeID := ensureOpenFGAStore(t)
	writeTestModel(t, storeID)

	// Create OpenFGA client
	fgaClient, err := NewClient(Config{APIURL: "http://localhost:18080", StoreID: storeID})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Connect to NATS and create KV bucket
	broker, err := events.Connect(context.Background(), events.ConnOptions{
		URL: "nats://localhost:14222",
	})
	if err != nil {
		t.Fatalf("NATS connection failed: %v", err)
	}
	defer broker.Close()

	kv, err := broker.EnsureKeyValue(events.DefaultKVConfig("authz_cache_test"))
	if err != nil {
		t.Fatalf("EnsureKeyValue failed: %v", err)
	}

	// Create cache backend with NATS KV
	backend := NewNATSKVBackend(
		func(key string) ([]byte, error) {
			entry, err := kv.Get(key)
			if err != nil {
				return nil, err
			}
			return entry.Value(), nil
		},
		func(key string, value []byte) error {
			_, err := kv.Put(key, value)
			return err
		},
	)

	cachedClient := NewCachedClient(fgaClient, &CacheConfig{
		Enabled: true,
		TTL:     time.Minute,
		Backend: backend,
	})

	// Write a tuple directly
	err = fgaClient.WriteTuple(ctx, "user:cached", "can_read", "document:cache-int-test")
	if err != nil {
		t.Fatalf("WriteTuple failed: %v", err)
	}
	t.Cleanup(func() { fgaClient.DeleteTuple(ctx, "user:cached", "can_read", "document:cache-int-test") })

	// First check — should hit OpenFGA, miss cache
	allowed, err := cachedClient.Check(ctx, CheckRequest{
		User: "user:cached", Relation: "can_read", Object: "document:cache-int-test",
	})
	if err != nil {
		t.Fatalf("first Check failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true on first check")
	}

	// Second check — should hit cache
	allowed, err = cachedClient.Check(ctx, CheckRequest{
		User: "user:cached", Relation: "can_read", Object: "document:cache-int-test",
	})
	if err != nil {
		t.Fatalf("second Check failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true on cached check")
	}
}
