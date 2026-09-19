package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

func handleMFAProtected(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"data": "sensitive-data"})
	}
}

func handleBlacklistProtected(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"data": "sensitive"})
	}
}

func handleNoCSRF(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"status": "no-csrf"})
	}
}

func handleRateLimited(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"status": "ok"})
	}
}

func handlePerUserLimited(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"status": "ok"})
	}
}

func handlePerKeyLimited(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"status": "ok"})
	}
}

func handlePerRoleLimited(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"status": "ok"})
	}
}

func handleViewerData(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"data": "viewer-only"})
	}
}

func handleMaxFuncLimited(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"status": "ok"})
	}
}
