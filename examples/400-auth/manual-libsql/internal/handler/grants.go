package handler

import (
	"time"

	"auth-roles/internal/auth"
	"auth-roles/internal/svc"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

// grantStore builds the delegated-grant store bound to the request pool.
func grantStore(c *runtime.RestCtx) *auth.SQLStore {
	return auth.NewSQLStore(svc.DB(c))
}

// handleListGrants lists grants where the caller is grantor or grantee.
func handleListGrants(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		grants, err := grantStore(c).ListGrants(a.UserID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"data": grants})
	}
}

// handleGrantPermission delegates a permission. A grantor can only grant what
// they hold themselves (no escalation).
func handleGrantPermission(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		var body struct {
			Grantee    string `json:"grantee"`
			Permission string `json:"permission"`
			Resource   string `json:"resource"`
			TTL        string `json:"ttl"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		if body.Grantee == "" || body.Permission == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "grantee and permission required"})
		}
		if !auth.CanDelegate(a.Roles, auth.ParseRolePermissions(svcCtx.RolePermissions), body.Permission) {
			return c.Status(403).JSON(runtime.Map{"code": 403, "message": "cannot grant a permission you do not have"})
		}
		var exp *time.Time
		if body.TTL != "" {
			if d, derr := time.ParseDuration(body.TTL); derr == nil {
				t := time.Now().Add(d)
				exp = &t
			}
		}
		id, err := grantStore(c).InsertGrant(a.UserID, body.Grantee, body.Permission, body.Resource, exp)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"granted": true, "id": id})
	}
}

// handleRevokeGrant revokes a grant owned by the caller.
func handleRevokeGrant(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		ok, err := grantStore(c).RevokeGrant(c.Params("id"), a.UserID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		if !ok {
			return c.Status(404).JSON(runtime.Map{"code": 404, "message": "grant not found"})
		}
		return c.JSON(runtime.Map{"revoked": true})
	}
}

// handleDebugItems is gated by the products:read permission, granted either by
// a role or by a delegated grant.
func handleDebugItems(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"items": []string{"debug-1", "debug-2"}})
	}
}

// handleCreateTeam creates a team in the caller's tenant.
func handleCreateTeam(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Name string `json:"name"`
		}
		if err := c.Bind(&body); err != nil || body.Name == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "name required"})
		}
		id := db.NewID()
		if _, err := svc.DB(c).Exec(c.Context(),
			`INSERT INTO auth_teams (id, name, created_at) VALUES ($1,$2, now())`, id, body.Name); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.Status(201).JSON(runtime.Map{"id": id, "name": body.Name})
	}
}

// handleListTeams lists the teams.
func handleListTeams(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		rows, err := svc.DB(c).Query(c.Context(), `SELECT id, name FROM auth_teams ORDER BY name`)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		defer rows.Close()
		type team struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		data := []team{}
		for rows.Next() {
			var t team
			if err := rows.Scan(&t.ID, &t.Name); err != nil {
				break
			}
			data = append(data, t)
		}
		return c.JSON(runtime.Map{"data": data})
	}
}
