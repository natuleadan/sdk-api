package runtime

import (
	"context"
	"testing"
	"time"
)

func TestSLOEvent_NoError(t *testing.T) {
	SLOEvent("test-slo", false)
}

func TestSLOEvent_Error(t *testing.T) {
	SLOEvent("test-slo", true)
}

func TestStartSLOMetricsCollector_Empty(t *testing.T) {
	startSLOMetricsCollector(context.Background(), nil)
}

func TestStartSLOMetricsCollector_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startSLOMetricsCollector(ctx, []SLOConfig{
		{Name: "api-availability", Target: 99.9, Window: 30 * 24 * time.Hour},
	})
}
