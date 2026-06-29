package db

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolConfig struct {
	URL               string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns:          defaultMaxConns(),
		MinConns:          2,
		MaxConnLifetime:   30 * time.Minute,
		MaxConnIdleTime:   5 * time.Minute,
		HealthCheckPeriod: 1 * time.Minute,
	}
}

func defaultMaxConns() int32 {
	if s := os.Getenv("PG_MAX_CONNS"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n > 0 {
			return int32(n)
		}
	}

	reserved := int32(10)
	if s := os.Getenv("PG_RESERVED_CONNS"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n >= 0 {
			reserved = int32(n)
		}
	}

	replicas := 1
	if s := os.Getenv("REPLICA_COUNT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			replicas = n
		}
	}

	maxConns := 100
	if s := os.Getenv("PG_SERVER_MAX_CONNS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxConns = n
		}
	}

	perPod := (maxConns - int(reserved)) / replicas
	return int32(math.Max(1, float64(perPod)))
}

func NewPool(ctx context.Context, cfg PoolConfig) (*pgxpool.Pool, error) {
	pgCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("db: parse url: %w", err)
	}

	if cfg.MaxConns > 0 {
		pgCfg.MaxConns = cfg.MaxConns
	} else {
		pgCfg.MaxConns = defaultMaxConns()
	}
	if cfg.MinConns > 0 {
		pgCfg.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		pgCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		pgCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	}
	if cfg.HealthCheckPeriod > 0 {
		pgCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping failed: %w", err)
	}

	return pool, nil
}
