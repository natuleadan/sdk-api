package handler

import (
	"tickets/internal/svc"
	"tickets/models"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/runtime"
)

func CreateOrder(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var req struct {
			TicketID int64 `json:"ticket_id"`
			Quantity int   `json:"quantity"`
		}
		if err := c.Bind(&req); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		if req.Quantity <= 0 {
			return c.Status(400).JSON(runtime.Map{"error": "quantity must be positive"})
		}

		ok, err := svcCtx.DecrementStock(c.Context(), req.TicketID, req.Quantity)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"error": "stock check failed"})
		}
		if !ok {
			return c.Status(409).JSON(runtime.Map{"error": "sold out"})
		}

		id, err := svcCtx.CreateOrder(c.Context(), req.TicketID, req.Quantity)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"error": "order failed"})
		}

		if pubErr := svcCtx.PublishEvent(c.Context(), models.OrderEvent{
			OrderID: id, TicketID: req.TicketID, Quantity: req.Quantity, Status: "created",
		}); pubErr != nil {
			logx.Errorf("publish event: %v", pubErr)
		}

		return c.Status(201).JSON(runtime.Map{"order_id": id, "status": "confirmed"})
	}
}
