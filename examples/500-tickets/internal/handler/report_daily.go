package handler

import (
	"tickets/internal/svc"

	"github.com/natuleadan/sdk-api/runtime"
)

func DailyReport(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		p := svcCtx.Pool()
		var sales int
		p.QueryRow(c.Context(), "SELECT COUNT(*) FROM orders WHERE status = 'confirmed'").Scan(&sales)
		var revenue float64
		p.QueryRow(c.Context(), "SELECT COALESCE(SUM(t.price * o.quantity), 0) FROM orders o JOIN tickets t ON t.id = o.ticket_id WHERE o.status = 'confirmed'").Scan(&revenue)
		return c.JSON(runtime.Map{"total_sales": sales, "total_revenue": revenue})
	}
}
