package main

import (
	"context"
	"log"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/runtime"
)

type OrderEvent struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Email  string `json:"email"`
}

func main() {
	svc, err := runtime.New("service.yaml")
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	// Health endpoint
	svc.WithRest("healthCheck", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "mode": "entry+exit"})
	})

	// Webhook receives orders, auto-publishes to NATS via nats_publish config
	svc.WithRest("onInboundOrder", func(c *fiber.Ctx) error {
		log.Printf("webhook received: %s", string(c.Body()))
		return c.JSON(fiber.Map{"received": true})
	})

	// Exit worker: send email when order is confirmed
	svc.WithExit("onSendEmail", func(ctx context.Context, msg []byte) ([]byte, error) {
		var order OrderEvent
		json.Unmarshal(msg, &order)
		log.Printf("EXIT [email-sender]: sending email to %s for order %s", order.Email, order.ID)
		// Simulate email sending
		return nil, nil
	})

	// Exit worker: validate order with reply
	svc.WithExit("onValidate", func(ctx context.Context, msg []byte) ([]byte, error) {
		var order OrderEvent
		json.Unmarshal(msg, &order)
		log.Printf("EXIT [order-validator]: validating order %s", order.ID)

		// Validation logic
		valid := order.Email != ""
		response, _ := json.Marshal(fiber.Map{
			"orderID": order.ID,
			"valid":   valid,
		})
		return response, nil
	})

	// Cron handler: daily report
	svc.WithCron("onDailyReport", func(ctx context.Context) error {
		log.Println("CRON [daily-report]: generating daily report...")
		return nil
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
