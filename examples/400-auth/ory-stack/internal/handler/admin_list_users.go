package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/runtime"
)

// handleListUsers lists identities from Ory Kratos (admin API). Identity is
// owned by Kratos, not a local users table.
func handleListUsers(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		if svcCtx.Ory == nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "Ory not configured"})
		}
		identities, err := svcCtx.Ory.ListIdentities(c.Context(), 250)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		users := make([]runtime.Map, 0, len(identities))
		for _, id := range identities {
			email, _ := id.Traits["email"].(string)
			users = append(users, runtime.Map{"id": id.ID, "email": email, "state": id.State})
		}
		return c.JSON(runtime.Map{"data": users})
	}
}
