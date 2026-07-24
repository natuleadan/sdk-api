package handler

import (
	"fmt"

	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

type ServiceContext struct{}

func NewServiceContext() *ServiceContext {
	return &ServiceContext{}
}

func GenerateReceipt(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			TransferID  string  `json:"transfer_id"`
			FromAccount string  `json:"from_account"`
			ToAccount   string  `json:"to_account"`
			Amount      float64 `json:"amount"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		a := c.Locals("auth").(*middleware.AuthContext)
		_ = a

		storageKey := fmt.Sprintf("receipts/%s.pdf", body.TransferID)

		store := c.Locals("storage")
		if store != nil {
			return c.Status(200).JSON(runtime.Map{
				"receipt_url": fmt.Sprintf("http://localhost:23605/api/v1/receipts/%s", storageKey),
				"transfer_id": body.TransferID,
				"from": body.FromAccount, "to": body.ToAccount,
				"amount": body.Amount,
			})
		}

		return c.Status(201).JSON(runtime.Map{
			"transfer_id": body.TransferID,
			"storage_key": storageKey,
		})
	}
}
