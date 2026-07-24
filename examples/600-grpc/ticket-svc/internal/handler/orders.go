package handler

import (
	authpb "600-grpc/pb/authpb"
	"600-grpc/ticket-svc/internal/models"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

type ServiceContext struct {
	svc    *runtime.Service
	orders *db.Table[models.Order]
}

func NewServiceContext(s *runtime.Service, orders *db.Table[models.Order]) *ServiceContext {
	return &ServiceContext{svc: s, orders: orders}
}

func BuyTicket(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			TicketID int64 `json:"ticket_id"`
			Quantity int   `json:"quantity"`
		}
		if err := c.Bind(&body); err != nil || body.Quantity <= 0 {
			return c.Status(400).JSON(runtime.Map{"error": "invalid request"})
		}
		a := c.Locals("auth").(*middleware.AuthContext)

		gc := svcCtx.svc.GetGRPCClient("auth-svc")
		if gc == nil {
			return c.Status(500).JSON(runtime.Map{"error": "gRPC unavailable"})
		}
		cr, err := authpb.NewAuthServiceClient(gc.Conn()).DeductCredit(c.Context(),
			&authpb.DeductCreditRequest{UserId: a.UserID, Amount: 1})
		if err != nil || !cr.Ok {
			return c.Status(402).JSON(runtime.Map{"error": "insufficient credits"})
		}

		order := models.Order{TicketID: body.TicketID, Quantity: body.Quantity, UserID: a.UserID, Status: "confirmed"}
		if err := svcCtx.orders.Create(c.Context(), &order); err != nil {
			return c.Status(500).JSON(runtime.Map{"error": "order failed"})
		}
		return c.Status(201).JSON(runtime.Map{"id": order.ID, "status": "confirmed"})
	}
}
