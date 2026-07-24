package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"tickets/internal/svc"
	"tickets/models"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/runtime"
)

func ProcessBatch(svcCtx *svc.ServiceContext) runtime.AsyncHandler {
	return func(body []byte, job *runtime.JobState) error {
		var req struct {
			TicketID int64 `json:"ticket_id"`
			Quantity int   `json:"quantity"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		if req.Quantity <= 0 {
			return fmt.Errorf("quantity must be positive")
		}
		if req.Quantity > 100 {
			return fmt.Errorf("quantity exceeds max (100)")
		}
		if req.TicketID <= 0 {
			return fmt.Errorf("invalid ticket_id")
		}

		results := make([]runtime.Map, 0, req.Quantity)
		for i := range req.Quantity {
			ok, err := svcCtx.DecrementStock(context.Background(), req.TicketID, 1)
			if err != nil {
				return err
			}
			if !ok {
				results = append(results, runtime.Map{"ticket": i + 1, "status": "sold_out"})
				continue
			}
			id, err := svcCtx.CreateOrder(context.Background(), req.TicketID, 1)
			if err != nil {
				return err
			}
			evt := models.OrderEvent{OrderID: id, TicketID: req.TicketID, Quantity: 1, Status: "batch.process"}
			if pubErr := svcCtx.PublishEvent(context.Background(), evt); pubErr != nil {
				logx.Errorf("batch publish: %v", pubErr)
			}
			results = append(results, runtime.Map{"ticket": i + 1, "status": "confirmed", "order_id": id})
		}

		job.Result = runtime.Map{"total": req.Quantity, "results": results}
		return nil
	}
}
