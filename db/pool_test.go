package db

import (
	"os"
	"testing"
)

func TestDefaultMaxConns(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected int32
	}{
		{
			name:     "default values (no env)",
			env:      map[string]string{},
			expected: 90,
		},
		{
			name: "PG_MAX_CONNS overrides",
			env: map[string]string{
				"PG_MAX_CONNS": "20",
			},
			expected: 20,
		},
		{
			name: "more replicas = fewer conns per pod",
			env: map[string]string{
				"REPLICA_COUNT": "3",
			},
			expected: 30,
		},
		{
			name: "custom server max conns with replicas",
			env: map[string]string{
				"PG_SERVER_MAX_CONNS": "200",
				"REPLICA_COUNT":       "5",
			},
			expected: 38,
		},
		{
			name: "minimum floor of 1",
			env: map[string]string{
				"PG_SERVER_MAX_CONNS": "20",
				"REPLICA_COUNT":       "20",
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				os.Setenv(k, v)
			}
			t.Cleanup(func() {
				for k := range tt.env {
					os.Unsetenv(k)
				}
			})

			got := defaultMaxConns()
			if got != tt.expected {
				t.Errorf("defaultMaxConns() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()
	if cfg.MaxConns != 90 {
		t.Errorf("expected 90, got %d", cfg.MaxConns)
	}
	if cfg.MinConns != 2 {
		t.Errorf("expected MinConns=2, got %d", cfg.MinConns)
	}
	if cfg.MaxConnLifetime == 0 {
		t.Errorf("expected non-zero MaxConnLifetime")
	}
}
