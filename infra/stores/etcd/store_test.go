package etcd

import (
	"testing"
)

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{
		Endpoints: []string{"localhost:2379"},
	}
	if len(cfg.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(cfg.Endpoints))
	}
	if cfg.Timeout != 0 {
		t.Errorf("expected 0 timeout, got %v", cfg.Timeout)
	}
	if cfg.Username != "" {
		t.Errorf("expected empty username, got %q", cfg.Username)
	}
}
