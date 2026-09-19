package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/natuleadan/sdk-api/db"
	sdkauth "github.com/natuleadan/sdk-api/runtime/auth"
)

// Grant is a delegated permission: the grantor allows the grantee to perform a
// permission on a resource until it expires (nil = no expiry). Status is
// "active" or "revoked".
type Grant struct {
	ID         string `json:"id"`
	Grantor    string `json:"grantor"`
	Grantee    string `json:"grantee"`
	Permission string `json:"permission"`
	Resource   string `json:"resource"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// RolePermissions maps a role to the permissions it natively holds. Permission
// strings may use trailing wildcards ("products:*", "teams:members:*").
type RolePermissions map[string][]string

// escalateErr is returned when a grantor tries to delegate beyond what they
// hold themselves.
var escalateErr = errors.New("cannot grant a permission you do not have")

// Store is the persistence boundary for grants, teams and memberships. It is
// expressed over an interface so the same logic runs on the SDK Querier (pg,
// turso, mysql) or any test double.
type Store interface {
	// InsertGrant persists a new active grant and returns its id.
	InsertGrant(grantor, grantee, permission, resource string, expiresAt *time.Time) (string, error)
	// ActiveGrants returns the grantee's active, non-expired grants.
	ActiveGrants(grantee string) ([]Grant, error)
	// RevokeGrant flips a grant to revoked, only if owned by grantor.
	RevokeGrant(id, grantor string) (bool, error)
	// ListGrants returns the grants where user is grantor or grantee.
	ListGrants(user string) ([]Grant, error)
}

// CanDelegate reports whether grantorRoles may grant permission. The grantor
// must natively hold a permission that covers the requested one, so no one can
// escalate beyond their own authority. The special roles "*" and "superadmin"
// are unrestricted.
func CanDelegate(grantorRoles []string, table RolePermissions, permission string) bool {
	for _, r := range grantorRoles {
		if r == "*" || r == "superadmin" {
			return true
		}
		if sdkauth.MatchAnyPermission(table[r], permission) {
			return true
		}
	}
	return false
}

// EffectivePermissions merges the permissions granted by the caller's roles
// with the active delegated grants, honoring wildcards in both. Returns the
// flattened set the caller effectively holds.
func EffectivePermissions(userID string, roles []string, table RolePermissions, store Store) ([]string, error) {
	set := map[string]bool{}
	for _, r := range roles {
		for _, p := range table[r] {
			set[p] = true
		}
	}
	if store != nil {
		grants, err := store.ActiveGrants(userID)
		if err != nil {
			return nil, err
		}
		for _, g := range grants {
			set[g.Permission] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out, nil
}

// Authorize checks required roles (with hierarchy) and permissions (with
// wildcards and delegated grants). Roles and permissions are alternatives:
// access is granted when either list is satisfied, matching the SDK entry
// semantics.
func Authorize(hierarchy sdkauth.RoleHierarchy, table RolePermissions, store Store, userID string, userRoles []string, requiredRoles, requiredPermissions []string) error {
	if len(requiredRoles) > 0 {
		for _, req := range requiredRoles {
			if req == "*" || hierarchy.Satisfies(userRoles, req) {
				return nil
			}
		}
	}
	if len(requiredPermissions) > 0 {
		perms, err := EffectivePermissions(userID, userRoles, table, store)
		if err != nil {
			return err
		}
		// A superadmin (or the "*" role) holds every permission.
		for _, role := range userRoles {
			if role == "*" || role == "superadmin" {
				return nil
			}
		}
		for _, req := range requiredPermissions {
			if sdkauth.MatchAnyPermission(perms, req) {
				return nil
			}
		}
	}
	if len(requiredRoles) == 0 && len(requiredPermissions) == 0 {
		return nil
	}
	return fmt.Errorf("access denied: roles=%v need_roles=%v need_permissions=%v",
		userRoles, requiredRoles, requiredPermissions)
}

// ParseRolePermissions turns a flat "role=perm1,perm2" list from the YAML into
// the table. Convenience for wiring; the example builds it from service.yaml.
func ParseRolePermissions(grants map[string][]string) RolePermissions {
	table := RolePermissions{}
	for role, perms := range grants {
		clean := make([]string, 0, len(perms))
		for _, p := range perms {
			if p = strings.TrimSpace(p); p != "" {
				clean = append(clean, p)
			}
		}
		table[role] = clean
	}
	return table
}

var _ = db.SQLDialect("")
