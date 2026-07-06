package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func initDatabases(ctx context.Context, databases []DBConfig) (map[string]any, error) {
	pools := make(map[string]any, len(databases))
	for i, dbCfg := range databases {
		if err := dbCfg.Validate(); err != nil {
			return nil, fmt.Errorf("databases[%d] (%s): %w", i, dbCfg.Name, err)
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
			pool, err = initMongo(&dbCfg)
		default:
			return nil, fmt.Errorf("unknown driver %q", dbCfg.Driver)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dbCfg.Name, err)
		}
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
	return db, nil
}

func initMySQL(cfg *DBConfig) (*sql.DB, error) {
	return db.MySQLOpen(cfg.URL)
}

// initMongo stores a MongoDB URI string in the pools map. The actual connection
// is lazily established by mon.MustNewModel on first CRUD access.
func initMongo(cfg *DBConfig) (string, error) {
	return cfg.URL, nil
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
