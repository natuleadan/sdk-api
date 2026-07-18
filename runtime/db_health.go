package runtime

import (
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolHealth struct {
	Name           string  `json:"name"`
	Driver         string  `json:"driver"`
	TotalConns     int     `json:"total_connections"`
	IdleConns      int     `json:"idle_connections"`
	InUseConns     int     `json:"in_use_connections"`
	MaxConns       int     `json:"max_connections"`
	UtilizationPct float64 `json:"utilization_pct"`
	WaitCount      int64   `json:"wait_count"`
	WaitDuration   string  `json:"wait_duration"`
	Status         string  `json:"status"`
}

func CheckPoolHealth(name, driver string, pool any) PoolHealth {
	h := PoolHealth{
		Name:   name,
		Driver: driver,
		Status: "healthy",
	}

	switch p := pool.(type) {
	case *pgxpool.Pool:
		s := p.Stat()
		h.TotalConns = int(s.TotalConns())
		h.IdleConns = int(s.IdleConns())
		h.InUseConns = int(s.AcquiredConns())
		h.MaxConns = int(s.MaxConns())
		h.WaitCount = s.AcquireCount()
		h.WaitDuration = s.AcquireDuration().Round(time.Millisecond).String()
		if h.MaxConns > 0 {
			h.UtilizationPct = float64(h.InUseConns) / float64(h.MaxConns) * 100
		}
	case *sql.DB:
		s := p.Stats()
		h.TotalConns = s.OpenConnections
		h.IdleConns = s.Idle
		h.InUseConns = s.InUse
		h.MaxConns = 0 // sql.DB doesn't expose max
		if h.TotalConns > 0 {
			h.UtilizationPct = float64(h.InUseConns) / float64(h.TotalConns) * 100
		}
		h.WaitCount = s.WaitCount
		h.WaitDuration = s.WaitDuration.Round(time.Millisecond).String()
	default:
		h.Status = "unknown"
		return h
	}

	if h.UtilizationPct > 80 {
		h.Status = "saturated"
	} else if h.UtilizationPct > 60 {
		h.Status = "degraded"
	}

	return h
}
