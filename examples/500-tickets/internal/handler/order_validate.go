package handler

import (
	"tickets/internal/svc"

	"github.com/natuleadan/sdk-api/runtime"
)

func ValidatePayment(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var req struct {
			Quantity int `json:"quantity"`
		}
		if err := c.Bind(&req); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		if req.Quantity > 5 {
			return c.Status(422).JSON(runtime.Map{"valid": false, "message": "quantity exceeds max per order (5)"})
		}
		return c.JSON(runtime.Map{"valid": true})
	}
}
