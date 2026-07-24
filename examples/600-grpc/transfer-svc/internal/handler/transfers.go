package handler

import (
	"context"
	"strconv"

	"600-grpc/transfer-svc/internal/models"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

type ServiceContext struct {
	svc       *runtime.Service
	transfers *db.Table[models.Transfer]
}

func NewServiceContext(s *runtime.Service, transfers *db.Table[models.Transfer]) *ServiceContext {
	return &ServiceContext{svc: s, transfers: transfers}
}

func InitiateTransfer(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			FromAccountID  string  `json:"from_account_id"`
			ToAccountID    string  `json:"to_account_id"`
			Amount         float64 `json:"amount"`
			Currency       string  `json:"currency"`
			IdempotencyKey string  `json:"idempotency_key"`
		}
		if err := c.Bind(&body); err != nil || body.Amount <= 0 {
			return c.Status(400).JSON(runtime.Map{"error": "invalid request"})
		}
		if body.Currency == "" {
			body.Currency = "USD"
		}

		fID, _ := strconv.ParseInt(body.FromAccountID, 10, 64)
		tID, _ := strconv.ParseInt(body.ToAccountID, 10, 64)
		transfer := models.Transfer{
			FromAccountID: fID, ToAccountID: tID,
			Amount: body.Amount, Currency: body.Currency, Status: "initiated",
			IdempotencyKey: body.IdempotencyKey,
		}
		if err := svcCtx.transfers.Create(c.Context(), &transfer); err != nil {
			return c.Status(500).JSON(runtime.Map{"error": "record failed"})
		}

		// Publish event via NATS
		broker := svcCtx.svc.NATS("default")
		if broker != nil {
			_ = broker.PublishJSON(c.Context(), "transfers.initiated", runtime.Map{
				"transfer_id":      transfer.ID,
				"from_account_id":  body.FromAccountID,
				"to_account_id":    body.ToAccountID,
				"amount":           body.Amount,
				"idempotency_key":  body.IdempotencyKey,
			})
		}

		return c.Status(201).JSON(runtime.Map{
			"transfer_id": transfer.ID, "status": "initiated",
			"from": body.FromAccountID, "to": body.ToAccountID,
			"amount": body.Amount,
		})
	}
}

func OnTransferCompleted(svcCtx *ServiceContext) func(ctx context.Context, msg []byte) ([]byte, error) {
	return func(ctx context.Context, msg []byte) ([]byte, error) {
		return nil, nil
	}
}
