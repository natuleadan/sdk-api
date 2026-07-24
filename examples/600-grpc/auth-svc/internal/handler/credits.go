package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func BuyCredits(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct{ Amount int `json:"amount"` }
		if err := c.Bind(&body); err != nil || body.Amount <= 0 {
			return c.Status(400).JSON(runtime.Map{"error": "invalid amount"})
		}
		a := c.Locals("auth").(*middleware.AuthContext)
		_, _ = c.PoolPG("primary").Exec(c.Context(),
			`UPDATE users SET credits = credits + $1 WHERE id = $2`, body.Amount, a.UserID)
		return c.JSON(runtime.Map{"added": body.Amount})
	}
}

func GetBalance(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := c.Locals("auth").(*middleware.AuthContext)
		var creds int
		_ = c.PoolPG("primary").QueryRow(c.Context(),
			`SELECT credits FROM users WHERE id = $1`, a.UserID).Scan(&creds)
		return c.JSON(runtime.Map{"credits": creds})
	}
}

func DeductCredits(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct{ Amount int `json:"amount"` }
		if err := c.Bind(&body); err != nil || body.Amount <= 0 {
			return c.Status(400).JSON(runtime.Map{"error": "invalid amount"})
		}
		a := c.Locals("auth").(*middleware.AuthContext)
		tag, err := c.PoolPG("primary").Exec(c.Context(),
			`UPDATE users SET credits = credits - $1 WHERE id = $2 AND credits >= $1`, body.Amount, a.UserID)
		if err != nil || tag.RowsAffected() == 0 {
			return c.Status(402).JSON(runtime.Map{"error": "insufficient credits"})
		}
		return c.JSON(runtime.Map{"deducted": body.Amount})
	}
}
