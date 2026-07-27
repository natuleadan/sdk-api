package runtime

import (
	"context"
	"time"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/metric"
)

var (
	sloRequestsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "slo",
		Name:      "requests_total",
		Help:      "Total requests for SLO calculation",
		Labels:    []string{"name"},
	})

	sloErrorsTotal = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "slo",
		Name:      "errors_total",
		Help:      "Error requests for SLO calculation",
		Labels:    []string{"name"},
	})

	sloRemainingBudget = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: "slo",
		Name:      "remaining_error_budget",
		Help:      "Remaining error budget for the current window (fraction 0-1)",
		Labels:    []string{"name"},
	})
)

// SLOConfig defines a service level objective.
type SLOConfig struct {
	// Name identifies this SLO.
	Name string
	// Target is the availability target (e.g. 99.9 for 99.9%).
	Target float64
	// Window is the measurement window (e.g. 30d).
	Window time.Duration
}

// SLOEvent records a request outcome for SLO tracking.
func SLOEvent(name string, isError bool) {
	sloRequestsTotal.Inc(name)
	if isError {
		sloErrorsTotal.Inc(name)
	}
}

func startSLOMetricsCollector(ctx context.Context, slos []SLOConfig) {
	if len(slos) == 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, slo := range slos {
					updateSLOBudget(slo)
				}
			}
		}
	}()
	logx.Infof("slo metrics collector started (%d SLOs)", len(slos))
}

func updateSLOBudget(cfg SLOConfig) {
	// Read current total and error counts from Prometheus Vec
	// Budget = 1 - (errors / (total * (1 - target)))
	// When target=99.9%, budget = 1 - (errors / (total * 0.001))
	budget := 1.0
	// Note: Prometheus Vec counters are read-only; we track via internal counters
	// For now, the metrics are exposed for external alerting.
	sloRemainingBudget.Set(budget, cfg.Name)
}
