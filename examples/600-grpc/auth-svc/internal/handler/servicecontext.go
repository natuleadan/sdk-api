package handler

import "os"

type ServiceContext struct {
	JWTSecret string
}

func NewServiceContext() *ServiceContext {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-hs256-change-in-prod"
	}
	return &ServiceContext{JWTSecret: secret}
}
