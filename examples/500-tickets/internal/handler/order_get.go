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
		order, err := svcCtx.OrderTable().Get(c.Context(), id)
		if err != nil {
			return c.Status(404).JSON(runtime.Map{"error": "not found"})
		}
		return c.JSON(runtime.Map{
			"id":        order.ID,
			"ticket_id": order.TicketID,
			"quantity":  order.Quantity,
			"status":    order.Status,
		})
	}
}
