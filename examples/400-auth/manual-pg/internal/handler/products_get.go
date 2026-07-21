package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleGetProduct(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return svcCtx.Store.Get(c)
	}
}
