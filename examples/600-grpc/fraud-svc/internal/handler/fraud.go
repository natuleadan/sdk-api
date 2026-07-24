package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
)

type ServiceContext struct{}

func NewServiceContext() *ServiceContext {
	return &ServiceContext{}
}

func CheckFraud(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Amount  float64 `json:"amount"`
			UserID  string  `json:"user_id"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		if body.Amount > 10000 {
			return c.JSON(runtime.Map{"fraud": true, "reason": "amount exceeds limit"})
		}
		return c.JSON(runtime.Map{"fraud": false})
	}
}
