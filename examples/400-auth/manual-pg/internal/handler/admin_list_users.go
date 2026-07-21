package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleListUsers(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		pool := c.PoolPG("primary")
		rows, err := pool.Query(c.Context(), `SELECT id, username, role FROM users ORDER BY username`)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		defer rows.Close()
		var users []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		for rows.Next() {
			var u struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Role     string `json:"role"`
			}
			if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
				break
			}
			users = append(users, u)
		}
		if users == nil {
			users = []struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Role     string `json:"role"`
			}{}
		}
		return c.JSON(runtime.Map{"data": users})
	}
}
