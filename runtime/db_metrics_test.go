package runtime

import (
	"context"
	"testing"
	"time"
)

func TestCollectPoolMetrics_Nil(t *testing.T) {
	collectPoolMetrics(nil, nil)
}

func TestCollectPoolMetrics_Empty(t *testing.T) {
	collectPoolMetrics(map[string]any{}, map[string]string{})
}

func TestCollectPoolMetrics_SQLitePool(t *testing.T) {
	pools := map[string]any{
		"test": struct{}{},
	}
	collectPoolMetrics(pools, map[string]string{"test": "turso"})
}

func TestStartPoolMetricsCollector_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pools := map[string]any{
		"primary": struct{}{},
	}
	dbs := []DBConfig{{Name: "primary", Driver: "postgres"}}
	startPoolMetricsCollector(ctx, pools, dbs)
	time.Sleep(50 * time.Millisecond)
	cancel()
}

func TestStartPoolMetricsCollector_NoPools(t *testing.T) {
	startPoolMetricsCollector(context.Background(), nil, nil)
}
