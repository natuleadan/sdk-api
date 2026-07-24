package handler

import (
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func Signup(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		if err := auth.CheckPasswordStrength(body.Password); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": err.Error()})
		}
		hash, _ := auth.HashPassword(body.Password)
		pool := c.PoolPG("primary")
		var id string
		err := pool.QueryRow(c.Context(),
			`INSERT INTO users (username, password, role, credits) VALUES ($1,$2,'viewer',15) RETURNING id`,
			body.Username, hash).Scan(&id)
		if err != nil {
			return c.Status(409).JSON(runtime.Map{"error": "username taken"})
		}
		return c.Status(201).JSON(runtime.Map{"id": id, "credits": 15})
	}
}

func Login(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		pool := c.PoolPG("primary")
		var id, hash, role string
		err := pool.QueryRow(c.Context(),
			`SELECT id, password, role FROM users WHERE username = $1`, body.Username).
			Scan(&id, &hash, &role)
		if err != nil || !auth.VerifyPassword(hash, body.Password) {
			return c.Status(401).JSON(runtime.Map{"error": "invalid credentials"})
		}
		claims := middleware.DefaultClaims(id, "", []string{role}, nil, 3600)
		signed, _ := middleware.SignToken(svcCtx.JWTSecret, "HS256", claims)
		return c.JSON(runtime.Map{"token": signed, "user_id": id})
	}
}


