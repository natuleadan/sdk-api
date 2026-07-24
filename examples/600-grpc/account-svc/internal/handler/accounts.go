package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

type ServiceContext struct {
	svc       *runtime.Service
	JWTSecret string
}

func NewServiceContext() *ServiceContext {
	return &ServiceContext{JWTSecret: "dev-secret"}
}

func (s *ServiceContext) SetService(svc *runtime.Service) {
	s.svc = svc
}

func CreateAccount(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct{ Currency string `json:"currency"` }
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		a := c.Locals("auth").(*middleware.AuthContext)
		cur := body.Currency
		if cur == "" {
			cur = "USD"
		}
		pool := c.PoolPG("primary")
		var id string
		err := pool.QueryRow(c.Context(),
			`INSERT INTO accounts (user_id, currency, balance, status) VALUES ($1,$2,1000.00,'active') RETURNING id`,
			a.UserID, cur).Scan(&id)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"error": "create failed"})
		}
		return c.Status(201).JSON(runtime.Map{"id": id, "currency": cur, "balance": 1000.00})
	}
}

func GetBalance(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := c.Locals("auth").(*middleware.AuthContext)
		pool := c.PoolPG("primary")
		var id string
		var balance float64
		err := pool.QueryRow(c.Context(),
			`SELECT id, balance FROM accounts WHERE id = $1 AND user_id = $2`,
			c.Params("id"), a.UserID).Scan(&id, &balance)
		if err != nil {
			return c.Status(404).JSON(runtime.Map{"error": "not found"})
		}
		return c.JSON(runtime.Map{"id": id, "balance": balance})
	}
}
