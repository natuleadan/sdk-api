package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func initDatabases(ctx context.Context, databases []DBConfig) (map[string]any, error) {
	pools := make(map[string]any, len(databases))
	poolByKey := make(map[string]any)
	for i, dbCfg := range databases {
		if err := dbCfg.Validate(); err != nil {
			return nil, fmt.Errorf("databases[%d] (%s): %w", i, dbCfg.Name, err)
		}
		dedupKey := dbCfg.Driver + ":" + dbCfg.URL
		if existing, ok := poolByKey[dedupKey]; ok {
			pools[dbCfg.Name] = existing
			logx.Infof("db %s: reuses pool for %s", dbCfg.Name, dedupKey)
			continue
		}
		var pool any
		var err error
		switch dbCfg.Driver {
		case "postgres":
			pool, err = initPostgres(ctx, &dbCfg)
		case "turso":
			pool, err = initTurso(&dbCfg)
		case "mysql":
			pool, err = initMySQL(&dbCfg)
		case "mongo":
			pool = initMongo(&dbCfg)
		default:
			return nil, fmt.Errorf("unknown driver %q", dbCfg.Driver)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dbCfg.Name, err)
		}
		poolByKey[dedupKey] = pool
		pools[dbCfg.Name] = pool
		logx.Infof("db connected: %s (%s)", dbCfg.Name, dbCfg.Driver)
	}
	return pools, nil
}

func initPostgres(ctx context.Context, cfg *DBConfig) (*pgxpool.Pool, error) {
	poolCfg := db.PoolConfig{
		URL: cfg.URL,
	}
	if cfg.Pool != nil {
		poolCfg.MaxConns = cfg.Pool.MaxConns
		poolCfg.MinConns = cfg.Pool.MinConns
		if d, err := time.ParseDuration(cfg.Pool.MaxConnLifetime); err == nil {
			poolCfg.MaxConnLifetime = d
		}
		if d, err := time.ParseDuration(cfg.Pool.MaxConnIdleTime); err == nil {
			poolCfg.MaxConnIdleTime = d
		}
		if d, err := time.ParseDuration(cfg.Pool.HealthCheckPeriod); err == nil {
			poolCfg.HealthCheckPeriod = d
		}
		// Auto-size: max(1, (PG_MAX_CONNS - RESERVED) / REPLICAS)
		if poolCfg.MaxConns <= 0 {
			pgMax := int32(100)
			reserved := cfg.Pool.ReservedConns
			replicas := int32(1)
			if v := os.Getenv("PG_SERVER_MAX_CONNS"); v != "" {
				if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
					pgMax = int32(n)
				}
			}
			if reserved <= 0 {
				reserved = 10
			}
			if v := os.Getenv("REPLICA_COUNT"); v != "" {
				if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
					replicas = int32(n)
				}
			}
			auto := max((pgMax-reserved)/replicas, 1)
			poolCfg.MaxConns = auto
		}
	} else {
		poolCfg.MaxConns = 10
		poolCfg.MinConns = 2
	}
	return db.NewPool(ctx, poolCfg)
}

func initTurso(cfg *DBConfig) (*sql.DB, error) {
	db, err := db.TursoOpen(cfg.URL)
	if err != nil {
		return nil, err
	}
	if cfg.Pool != nil && cfg.Pool.MaxConns > 0 {
		db.SetMaxOpenConns(int(cfg.Pool.MaxConns))
	}
	if cfg.Turso != nil && cfg.Turso.Mode == "local" && cfg.Turso.BusyTimeout > 0 {
		conn, err := db.Conn(context.Background())
		if err == nil {
			_, _ = conn.ExecContext(context.Background(),
				fmt.Sprintf("PRAGMA busy_timeout = %d", cfg.Turso.BusyTimeout))
			if err := conn.Close(); err != nil {
				logx.Errorf("close turso conn: %v", err)
			}
		}
	}
	return db, nil
}

func initMySQL(cfg *DBConfig) (*sql.DB, error) {
	db, err := db.MySQLOpen(cfg.URL)
	if err != nil {
		return nil, err
	}
	if cfg.Pool != nil && cfg.Pool.MaxConns > 0 {
		db.SetMaxOpenConns(int(cfg.Pool.MaxConns))
		if cfg.Pool.MinConns > 0 {
			db.SetMaxIdleConns(int(cfg.Pool.MinConns))
		}
	}
	return db, nil
}

// initMongo stores a MongoDB URI string in the pools map with optional pool params.
func initMongo(cfg *DBConfig) string {
	uri := cfg.URL
	if cfg.Pool != nil && cfg.Pool.MaxConns > 0 {
		if !strings.Contains(uri, "/?") && !strings.Contains(uri, "?") {
			uri += "/"
		}
		uri = fmt.Sprintf("%s?maxPoolSize=%d&maxConnecting=%d", uri, cfg.Pool.MaxConns, min(cfg.Pool.MaxConns/10, 10))
	}
	return uri
}

// Pool returns a pool by name. Returns nil if not found or not the expected type.
func Pool(pools map[string]any, name string) any {
	return pools[name]
}

// PoolPG returns a *pgxpool.Pool by name.
func PoolPG(pools map[string]any, name string) *pgxpool.Pool {
	if p, ok := pools[name].(*pgxpool.Pool); ok {
		return p
	}
	return nil
}

// PoolSQL returns a *sql.DB by name (for Turso or MySQL).
func PoolSQL(pools map[string]any, name string) *sql.DB {
	if d, ok := pools[name].(*sql.DB); ok {
		return d
	}
	return nil
}

// TableFor creates a db.Table[T] for the given pool name and table name.
func TableFor[T any](pools map[string]any, poolName, tableName string) (*db.Table[T], error) {
	pool := PoolPG(pools, poolName)
	if pool == nil {
		return nil, fmt.Errorf("pool %q not found or not postgres", poolName)
	}
	return db.NewTable[T](pool, tableName)
}
