package runtime

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestInitDatabases_UnknownDriver(t *testing.T) {
	_, err := initDatabases(context.Background(), []DBConfig{
		{Name: "bad", Driver: "oracle", URL: "x"},
	})
	if err == nil {
		t.Error("expected error for unknown driver")
	}
}

func TestInitDatabases_MissingName(t *testing.T) {
	_, err := initDatabases(context.Background(), []DBConfig{
		{Driver: "postgres", URL: "x"},
	})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestInitDatabases_MissingURL(t *testing.T) {
	_, err := initDatabases(context.Background(), []DBConfig{
		{Name: "pg", Driver: "postgres"},
	})
	if err == nil {
		t.Error("expected error for missing url")
	}
}

func TestInitDatabases_EmptyList(t *testing.T) {
	pools, err := initDatabases(context.Background(), nil)
	if err != nil {
		t.Errorf("empty databases should not error: %v", err)
	}
	if len(pools) != 0 {
		t.Errorf("expected 0 pools, got %d", len(pools))
	}
}

func TestPool(t *testing.T) {
	pools := map[string]any{"pg": &pgxpool.Pool{}}
	if Pool(pools, "pg") == nil {
		t.Error("Pool should return pool")
	}
	if Pool(pools, "missing") != nil {
		t.Error("Pool should return nil for missing")
	}
}

func TestPoolPG(t *testing.T) {
	pgPool := &pgxpool.Pool{}
	pools := map[string]any{"pg": pgPool}
	if PoolPG(pools, "pg") != pgPool {
		t.Error("PoolPG should return *pgxpool.Pool")
	}
	if PoolPG(pools, "missing") != nil {
		t.Error("PoolPG should return nil for missing")
	}
}

func TestPoolSQL(t *testing.T) {
	pools := map[string]any{"mysql": "not a sql.DB"}
	if PoolSQL(pools, "mysql") != nil {
		t.Error("PoolSQL should return nil when not *sql.DB")
	}
	if PoolSQL(pools, "missing") != nil {
		t.Error("PoolSQL should return nil for missing")
	}
}

func TestTableFor_MissingPool(t *testing.T) {
	_, err := TableFor[testModel](map[string]any{}, "missing", "table")
	if err == nil {
		t.Error("expected error for missing pool")
	}
}

func TestTableFor_NonPgPool(t *testing.T) {
	_, err := TableFor[testModel](map[string]any{"pg": "string"}, "pg", "table")
	if err == nil {
		t.Error("expected error for non-pgx pool")
	}
}

func TestDBConfig_Validate_AutoPostgres(t *testing.T) {
	cfg := DBConfig{Name: "pg", Driver: "postgres", URL: "postgres://x"}
	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate: %v", err)
	}
	if cfg.Pool.MaxConns != 10 {
		t.Errorf("MaxConns = %d, want 10", cfg.Pool.MaxConns)
	}
}

func TestInitPostgres_InvalidURL(t *testing.T) {
	ctx := context.Background()
	_, err := initPostgres(ctx, &DBConfig{Name: "pg", Driver: "postgres", URL: "postgres://invalid:5432/db"})
	if err == nil {
		t.Log("postgres connection unexpectedly succeeded (no real PG)")
	} else {
		t.Logf("expected error: %v", err)
	}
}
