package zitadel

import (
	"context"
	"net"
	"testing"
	"time"
)

func skipIfNoZitadel(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", "localhost:18082")
	if err != nil {
		t.Skipf("Zitadel not available at localhost:18082: %v", err)
	}
	conn.Close()
}

func TestIntegration_Zitadel_JWKSFetch(t *testing.T) {
	skipIfNoZitadel(t)

	client := NewClient(Config{Issuer: "http://localhost:18082"})

	keys, err := client.getKeys(context.Background())
	if err != nil {
		t.Fatalf("getKeys failed: %v", err)
	}

	if len(keys) == 0 {
		t.Fatal("expected at least one JWKS key from Zitadel")
	}

	for kid, key := range keys {
		t.Logf("key found: kid=%s, N bits=%d", kid, key.N.BitLen())
		if key.N.BitLen() < 2048 {
			t.Errorf("key %s has fewer than 2048 bits: %d", kid, key.N.BitLen())
		}
	}
}

func TestIntegration_Zitadel_CacheExpiry(t *testing.T) {
	skipIfNoZitadel(t)

	client := NewClient(Config{Issuer: "http://localhost:18082", TTL: 10 * time.Minute})

	keys, err := client.getKeys(context.Background())
	if err != nil {
		t.Fatalf("first getKeys failed: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected keys on first fetch")
	}

	// Second call should use cache
	cachedKeys, err := client.getKeys(context.Background())
	if err != nil {
		t.Fatalf("cached getKeys failed: %v", err)
	}
	if len(cachedKeys) != len(keys) {
		t.Errorf("cached keys count mismatch: %d vs %d", len(cachedKeys), len(keys))
	}
}
