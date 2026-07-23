package handler

import (
	"net/http"

	"tickets/internal/svc"

	"github.com/natuleadan/sdk-api/runtime"
)

func BatchCompleteWebhook(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		body := c.Body()
		sig := c.Get("X-Job-Signature")

		expected := svcCtx.ExpectedSignature(body)
		if sig != "" && sig != expected {
			return c.Status(http.StatusUnauthorized).JSON(runtime.Map{"error": "invalid signature"})
		}

		svcCtx.RecordCallback(body)
		return c.JSON(runtime.Map{"status": "ok"})
	}
}
