package handler

import (
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

// handleGrantRole delegates a role by writing a Keto tuple
// (roles:<role>#assignee@user:<grantee>). Keto is the authorization source of
// truth, so a delegated role takes effect immediately.
func handleGrantRole(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		var body struct {
			Grantee string `json:"grantee"`
			Role    string `json:"role"`
		}
		if err := c.Bind(&body); err != nil || body.Grantee == "" || body.Role == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "grantee and role required"})
		}
		if svcCtx.Ory == nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "Ory not configured"})
		}
		if err := svcCtx.Ory.WriteKetoTuple(c.Context(), "roles", body.Role, "assignee", "user:"+body.Grantee); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"granted": true, "grantee": body.Grantee, "role": body.Role})
	}
}

// handleRevokeRole removes a delegated role (Keto tuple).
func handleRevokeRole(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		if getAuth(c) == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		if svcCtx.Ory == nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "Ory not configured"})
		}
		if err := svcCtx.Ory.DeleteKetoTuple(c.Context(), "roles", c.Params("role"), "assignee", "user:"+c.Params("id")); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"revoked": true})
	}
}

// handleMyRoles reports the roles the caller holds according to Keto, by
// checking each known role.
func handleMyRoles(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		if svcCtx.Ory == nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "Ory not configured"})
		}
		subject := "user:" + a.UserID
		roles := []string{}
		for _, role := range []string{"admin", "editor", "viewer", "facturacion-lectura"} {
			ok, err := svcCtx.Ory.CheckPermission(c.Context(), "roles", role, "assignee", subject)
			if err != nil {
				return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
			}
			if ok {
				roles = append(roles, role)
			}
		}
		return c.JSON(runtime.Map{"data": roles})
	}
}

// handleCreateTeam is a Keto-backed placeholder that records a team object;
// membership would be a roles-style subject set in a full deployment.
func handleCreateTeam(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			Name string `json:"name"`
		}
		if err := c.Bind(&body); err != nil || body.Name == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "name required"})
		}
		id := db.NewID()
		if _, err := c.PoolPG("primary").Exec(c.Context(),
			`INSERT INTO auth_teams (id, name, created_at) VALUES ($1,$2, now())`, id, body.Name); err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.Status(201).JSON(runtime.Map{"id": id, "name": body.Name})
	}
}

// handleListTeams lists the teams.
func handleListTeams(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		rows, err := c.PoolPG("primary").Query(c.Context(), `SELECT id, name FROM auth_teams ORDER BY name`)
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
