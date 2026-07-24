package handler

import (
	"context"
	"encoding/json"

	"github.com/natuleadan/sdk-api/runtime"
)

func (s *ServiceContext) publishEvent(ctx context.Context, subject string, data any) {
	if s.svc == nil {
		return
	}
	broker := s.svc.NATS("default")
	if broker != nil {
		_ = broker.PublishJSON(ctx, subject, data)
	}
}

func OnTransferInitiated(svcCtx *ServiceContext) func(ctx context.Context, msg []byte) ([]byte, error) {
	return func(ctx context.Context, msg []byte) ([]byte, error) {
		var evt struct {
			FromAccountID string  `json:"from_account_id"`
			ToAccountID   string  `json:"to_account_id"`
			Amount        float64 `json:"amount"`
			TransferID    int64   `json:"transfer_id"`
		}
		if err := json.Unmarshal(msg, &evt); err != nil {
			return nil, err
		}

		pool := svcCtx.svc.PoolPGTyped("primary")
		if pool == nil {
			return nil, nil
		}

		tag, err := pool.Exec(ctx,
			`UPDATE accounts SET balance = balance - $1 WHERE id = $2 AND balance >= $1 AND status = 'active'`,
			evt.Amount, evt.FromAccountID)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			svcCtx.publishEvent(ctx, "transfers.failed", runtime.Map{
				"transfer_id": evt.TransferID, "reason": "insufficient funds",
			})
			return nil, nil
		}

		pool.Exec(ctx,
			`UPDATE accounts SET balance = balance + $1 WHERE id = $2 AND status = 'active'`,
			evt.Amount, evt.ToAccountID)

		svcCtx.publishEvent(ctx, "transfers.completed", runtime.Map{
			"transfer_id": evt.TransferID,
			"from":        evt.FromAccountID, "to": evt.ToAccountID,
			"amount": evt.Amount,
		})
		return nil, nil
	}
}
