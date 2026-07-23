package handler

import (
	"strconv"

	"tickets/internal/svc"

	"github.com/natuleadan/sdk-api/runtime"
)

func GetOrder(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		idStr := c.Query("id")
		if idStr == "" {
			return c.Status(400).JSON(runtime.Map{"error": "id query required"})
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid id"})
		}
		p := svcCtx.Pool()
		var ticketID, quantity int64
		var status string
		if err := p.QueryRow(c.Context(), "SELECT ticket_id, quantity, status FROM orders WHERE id = $1", id).Scan(&ticketID, &quantity, &status); err != nil {
			return c.Status(404).JSON(runtime.Map{"error": "not found"})
		}
		return c.JSON(runtime.Map{"id": id, "ticket_id": ticketID, "quantity": quantity, "status": status})
	}
}
