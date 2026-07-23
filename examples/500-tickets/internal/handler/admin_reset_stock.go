package handler

import (
	"tickets/internal/svc"

	"github.com/natuleadan/sdk-api/runtime"
)

func ResetStock(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var req struct {
			TicketID int64 `json:"ticket_id"`
			Stock    int   `json:"stock"`
		}
		if err := c.Bind(&req); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		if err := svcCtx.ResetStock(c.Context(), req.TicketID, req.Stock); err != nil {
			return c.Status(500).JSON(runtime.Map{"error": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "ok"})
	}
}
