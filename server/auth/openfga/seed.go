package openfga

import (
	"context"
	"fmt"
)

// PermissionDef defines a role-to-actions mapping for seeding.
type PermissionDef struct {
	Role     string   `json:"role"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

// SeedPermissions seeds role-permission tuples into OpenFGA.
// Idempotent: safe to call on every startup.
// Each action becomes a tuple: role:<role>#can_<action> → <resource>:<action>.
func (c *Client) SeedPermissions(ctx context.Context, permissions []PermissionDef) error {
	for _, p := range permissions {
		for _, action := range p.Actions {
			user := fmt.Sprintf("role:%s", p.Role)
			relation := fmt.Sprintf("can_%s", action)
			object := fmt.Sprintf("%s:%s", p.Resource, action)

			if err := c.WriteTuple(ctx, user, relation, object); err != nil {
				return fmt.Errorf("seed: write %s %s %s: %w", user, relation, object, err)
			}
		}
	}
	return nil
}

// DefaultPermissions returns a sensible default set of permissions
// matching the CRUD entry pattern.
func DefaultPermissions(resource string) []PermissionDef {
	return []PermissionDef{
		{
			Role:     fmt.Sprintf("%s:manager", resource),
			Resource: resource,
			Actions:  []string{"create", "read", "update", "delete", "publish"},
		},
		{
			Role:     fmt.Sprintf("%s:editor", resource),
			Resource: resource,
			Actions:  []string{"create", "read", "update"},
		},
		{
			Role:     fmt.Sprintf("%s:viewer", resource),
			Resource: resource,
			Actions:  []string{"read"},
		},
	}
}
