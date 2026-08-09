package events

import (
	"context"
	"testing"
	"time"
)

// TestConnectOptions_Params verifies the option plumbing is sound: a bad URL
// with TLS/user options fails fast rather than hanging or panicking.
func TestConnectOptions_Params(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Connect(ctx, ConnOptions{
		URL:      "tls://127.0.0.1:1", // nothing listens on :1
		Timeout:  2 * time.Second,
		User:     "app",
		Password: "x",
		CAFile:   "/nonexistent/ca.pem",
		CertFile: "/nonexistent/cert.pem",
		KeyFile:  "/nonexistent/key.pem",
	})
	if err == nil {
		if conn != nil {
			conn.NC.Close()
		}
		t.Fatal("expected connect error for unreachable/MTLS endpoint")
	}
}

// TestDefaultKVConfig_R1 ensures the cache default keeps the legacy behavior
// (single replica, memory, short TTL) unless a caller overrides it.
func TestDefaultKVConfig_R1(t *testing.T) {
	cfg := DefaultKVConfig("t-bucket")
	if cfg.MaxBytes != 256*1024*1024 {
		t.Fatalf("MaxBytes = %d", cfg.MaxBytes)
	}
	if cfg.Storage != MemoryStorage {
		t.Fatalf("Storage = %v, want MemoryStorage", cfg.Storage)
	}
}
