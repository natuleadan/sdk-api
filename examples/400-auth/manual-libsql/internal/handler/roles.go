package handler

import (
	"context"
	"time"

	"auth-roles/internal/auth"
	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

// ValidateRolesDB resolves the caller's roles from the database (dynamic,
// arbitrary names, expiry- and status-aware) plus the JWT claims for backward
// compatibility, then authorizes required roles (hierarchy) and permissions
// (wildcards + delegated grants).
func ValidateRolesDB(pool *svc.Querier, table auth.RolePermissions, ctx context.Context, a *middleware.AuthContext, requiredRoles, requiredPermissions []string) error {
	roles, _, err := resolveRolesPermissions(ctx, pool, a.UserID, a.Roles, a.Permissions)
	if err != nil {
		return err
	}
	store := auth.NewSQLStore(pool)
	return auth.Authorize(roleHierarchy, table, store, a.UserID, roles, requiredRoles, requiredPermissions)
}

// resolveRolesPermissions merges claim roles/permissions with the legacy
// users.role column and the role_assignments/role_permissions tables. Only
// active, non-expired assignments count.
func resolveRolesPermissions(ctx context.Context, pool *svc.Querier, userID string, claimRoles, claimPerms []string) ([]string, []string, error) {
	roleSet := map[string]bool{}
	for _, r := range claimRoles {
		if r != "" {
			roleSet[r] = true
		}
	}
	var legacy string
	if err := pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&legacy); err == nil && legacy != "" {
		roleSet[legacy] = true
	}

	rows, err := pool.Query(ctx,
		`SELECT role FROM role_assignments
		 WHERE user_id = $1 AND status = 'active' AND (expires_at IS NULL OR expires_at > now())`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, nil, err
		}
		if r != "" {
			roleSet[r] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	roles := make([]string, 0, len(roleSet))
	for r := range roleSet {
		roles = append(roles, r)
	}

	permSet := map[string]bool{}
	for _, p := range claimPerms {
		if p != "" {
			permSet[p] = true
		}
	}
	if len(roles) > 0 {
		pred, inArgs := db.InList(pool.Dialect(), "role", roles)
		prows, err := pool.Query(ctx, `SELECT permission FROM role_permissions WHERE `+pred, inArgs...)
		if err != nil {
			return nil, nil, err
		}
		defer prows.Close()
		for prows.Next() {
			var p string
			if err := prows.Scan(&p); err != nil {
				return nil, nil, err
			}
			if p != "" {
				permSet[p] = true
			}
		}
		if err := prows.Err(); err != nil {
			return nil, nil, err
		}
	}

	perms := make([]string, 0, len(permSet))
	for p := range permSet {
		perms = append(perms, p)
	}
	return roles, perms, nil
}

// handleAssignRole grants or requests a role. Body: {role, status?, expires_at?}
// (status: active|pending; expires_at: RFC3339). Idempotent per (user, role).
func handleAssignRole(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		if getAuth(c) == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		id := c.Params("id")
		var body struct {
			Role      string `json:"role"`
			Status    string `json:"status"`
			ExpiresAt string `json:"expires_at"`
		}
		if err := c.Bind(&body); err != nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "invalid body"})
		}
		if body.Role == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "role required"})
		}
		status := body.Status
		if status == "" {
			status = "active"
		}
		if status != "active" && status != "pending" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "status must be active or pending"})
		}
		var expires any
		if body.ExpiresAt != "" {
			t, perr := time.Parse(time.RFC3339, body.ExpiresAt)
			if perr != nil {
				return c.Status(400).JSON(runtime.Map{"code": 400, "message": "expires_at must be RFC3339"})
			}
			expires = t
		}
		pool := svc.DB(c)
		_, err := pool.Exec(c.Context(),
			`INSERT INTO role_assignments (user_id, role, status, expires_at, granted_at, accepted_at)
			 VALUES ($1,$2,$3,$4, now(), CASE WHEN $3 = 'active' THEN now() ELSE NULL END)
			 ON CONFLICT (user_id, role) DO UPDATE SET
			   status = EXCLUDED.status, expires_at = EXCLUDED.expires_at,
			   granted_at = now(), accepted_at = EXCLUDED.accepted_at`,
			id, body.Role, status, expires)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "assigned", "role": body.Role, "state": status})
	}
}

// handleRevokeRole marks a role assignment as revoked.
func handleRevokeRole(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		if getAuth(c) == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		pool := svc.DB(c)
		tag, err := pool.Exec(c.Context(),
			`UPDATE role_assignments SET status = 'revoked' WHERE user_id = $1 AND role = $2`,
			c.Params("id"), c.Params("role"))
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "revoked", "rows": tag.RowsAffected()})
	}
}

// handleMyRoles lists the caller's role assignments with an effective flag.
func handleMyRoles(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		pool := svc.DB(c)
		rows, err := pool.Query(c.Context(),
			`SELECT role, status, expires_at, accepted_at,
			        (status = 'active' AND (expires_at IS NULL OR expires_at > now())) AS effective
			 FROM role_assignments WHERE user_id = $1 ORDER BY role`, a.UserID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		defer rows.Close()
		type row struct {
			Role      string     `json:"role"`
			Status    string     `json:"status"`
			ExpiresAt *time.Time `json:"expires_at"`
			Accepted  *time.Time `json:"accepted_at"`
			Effective bool       `json:"effective"`
		}
		data := []row{}
		for rows.Next() {
			var r row
			var expRaw, accRaw any
			if err := rows.Scan(&r.Role, &r.Status, &expRaw, &accRaw, &r.Effective); err != nil {
				return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
			}
			r.ExpiresAt, _ = db.ParseTimePtr(expRaw)
			r.Accepted, _ = db.ParseTimePtr(accRaw)
			data = append(data, r)
		}
		return c.JSON(runtime.Map{"data": data})
	}
}

// handleAcceptRole activates a pending role assignment for the caller.
func handleAcceptRole(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		pool := svc.DB(c)
		tag, err := pool.Exec(c.Context(),
			`UPDATE role_assignments SET status = 'active', accepted_at = now()
			 WHERE user_id = $1 AND role = $2 AND status = 'pending'`,
			a.UserID, c.Params("role"))
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		if tag.RowsAffected() == 0 {
			return c.Status(404).JSON(runtime.Map{"code": 404, "message": "no pending role"})
		}
		return c.JSON(runtime.Map{"status": "accepted", "role": c.Params("role")})
	}
}

// handleFacturacion is gated by an arbitrary role name (facturacion-lectura).
func handleFacturacion(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"ok": true, "report": "invoices"})
	}
}

// handleManageUsers is gated by the users:manage permission alone (no role),
// which a role reaches through the role_permissions table.
func handleManageUsers(_ *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.JSON(runtime.Map{"ok": true, "scope": "users:manage"})
	}
}
