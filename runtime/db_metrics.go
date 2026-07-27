package runtime

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/metric"
)

var (
	dbPoolMaxConns = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "db_pool",
		Name:      "max_connections",
		Help:      "Maximum number of pool connections",
		Labels:    []string{"name", "driver"},
	})
	dbPoolIdleConns = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "db_pool",
		Name:      "idle_connections",
		Help:      "Current number of idle connections",
		Labels:    []string{"name", "driver"},
	})
	dbPoolInUseConns = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "db_pool",
		Name:      "in_use_connections",
		Help:      "Current number of in-use connections",
		Labels:    []string{"name", "driver"},
	})
	dbPoolTotalConns = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "db_pool",
		Name:      "total_connections",
		Help:      "Current total number of connections",
		Labels:    []string{"name", "driver"},
	})
	dbPoolAcquireDuration = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "db_pool",
		Name:      "acquire_duration_seconds",
		Help:      "Average connection acquire duration in seconds",
		Labels:    []string{"name", "driver"},
	})
	dbPoolEmptyAcquireCount = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "db_pool",
		Name:      "empty_acquire_total",
		Help:      "Number of times acquire returned empty (pool exhausted)",
		Labels:    []string{"name", "driver"},
	})
)

func startPoolMetricsCollector(ctx context.Context, pools map[string]any, dbs []DBConfig) {
	if len(pools) == 0 {
		return
	}
	driverMap := make(map[string]string, len(dbs))
	for _, db := range dbs {
		driverMap[db.Name] = db.Driver
	}

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectPoolMetrics(pools, driverMap)
			}
		}
	}()
	logx.Info("pool metrics collector started (15s interval)")
}

func collectPoolMetrics(pools map[string]any, drivers map[string]string) {
	for name, pool := range pools {
		driver := drivers[name]
		if driver == "" {
			driver = "postgres"
		}
		switch p := pool.(type) {
		case *pgxpool.Pool:
			s := p.Stat()
			dbPoolMaxConns.Set(float64(s.MaxConns()), name, driver)
			dbPoolIdleConns.Set(float64(s.IdleConns()), name, driver)
			dbPoolInUseConns.Set(float64(s.AcquiredConns()), name, driver)
			dbPoolTotalConns.Set(float64(s.TotalConns()), name, driver)
			dbPoolAcquireDuration.Set(s.AcquireDuration().Seconds(), name, driver)
			dbPoolEmptyAcquireCount.Set(float64(s.EmptyAcquireCount()), name, driver)
		case *sql.DB:
			s := p.Stats()
			dbPoolTotalConns.Set(float64(s.OpenConnections), name, driver)
			dbPoolIdleConns.Set(float64(s.Idle), name, driver)
			dbPoolInUseConns.Set(float64(s.InUse), name, driver)
			dbPoolEmptyAcquireCount.Set(float64(s.WaitCount), name, driver)
			dbPoolAcquireDuration.Set(s.WaitDuration.Seconds(), name, driver)
		}
	}
}
