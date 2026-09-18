package openfga

import (
	"context"
	"fmt"
	"slices"

	openfga "github.com/openfga/go-sdk"
)

// DefaultActions lists the actions granted by DefaultPermissions.
var DefaultActions = []string{"create", "read", "update", "delete", "publish"}

// PermissionDef defines a role-to-actions mapping for seeding.
type PermissionDef struct {
	Role     string   `json:"role"`
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

// SeedPermissions seeds role-permission tuples into OpenFGA.
// Idempotent: safe to call on every startup.
// Each action becomes a userset tuple: role:<role>#member → can_<action> → <resource>:<action>,
// so every member of the role inherits the permission.
func (c *Client) SeedPermissions(ctx context.Context, permissions []PermissionDef) error {
	for _, p := range permissions {
		for _, action := range p.Actions {
			user := fmt.Sprintf("role:%s#member", p.Role)
			relation := fmt.Sprintf("can_%s", action)
			object := fmt.Sprintf("%s:%s", p.Resource, action)

			if err := c.WriteTuple(ctx, user, relation, object); err != nil {
				return fmt.Errorf("seed: write %s %s %s: %w", user, relation, object, err)
			}
		}
	}
	return nil
}

// AssignRole makes a user a member of a role (user → member → role:<role>).
func (c *Client) AssignRole(ctx context.Context, user, role string) error {
	return c.WriteTuple(ctx, user, "member", fmt.Sprintf("role:%s", role))
}

// ResourceActions maps a resource type to the actions that must exist as
// `can_<action>` relations on it.
type ResourceActions map[string][]string

// EnsureModel derives the authorization model from the collected permissions
// and writes it. The base types (`user`, `role`) are always written so the role
// gate works even with no permissions; resources and actions come from the
// permissions themselves, so custom actions (e.g. `users:manage`) are covered.
func (c *Client) EnsureModel(ctx context.Context, permissions []PermissionDef) (string, error) {
	resources := make(ResourceActions)
	for _, p := range permissions {
		if p.Resource == "" {
			continue
		}
		for _, action := range p.Actions {
			if !slices.Contains(resources[p.Resource], action) {
				resources[p.Resource] = append(resources[p.Resource], action)
			}
		}
	}
	return c.EnsureDefaultModel(ctx, resources)
}

// EnsureDefaultModel writes the authorization model matching the SDK
// conventions: a `user` that is `member` of `role:<name>` inherits
// `can_<action>` on `<resource>:<action>`. OpenFGA uses the latest model, so
// this is safe to call on every startup; it returns the new model ID.
func (c *Client) EnsureDefaultModel(ctx context.Context, resources ResourceActions) (string, error) {
	defs := []openfga.TypeDefinition{
		{Type: "user"},
		{
			Type: "role",
			Relations: &map[string]openfga.Userset{
				"member": {This: &map[string]any{}},
			},
			Metadata: &openfga.Metadata{Relations: &map[string]openfga.RelationMetadata{
				"member": {DirectlyRelatedUserTypes: &[]openfga.RelationReference{relationRef("user", "")}},
			}},
		},
	}
	for resource, actions := range resources {
		relations := make(map[string]openfga.Userset, len(actions))
		meta := make(map[string]openfga.RelationMetadata, len(actions))
		for _, action := range actions {
			rel := "can_" + action
			relations[rel] = openfga.Userset{This: &map[string]any{}}
			meta[rel] = openfga.RelationMetadata{DirectlyRelatedUserTypes: &[]openfga.RelationReference{
				relationRef("role", "member"),
				relationRef("user", ""),
			}}
		}
		defs = append(defs, openfga.TypeDefinition{
			Type:      resource,
			Relations: &relations,
			Metadata:  &openfga.Metadata{Relations: &meta},
		})
	}
	return c.WriteAuthorizationModel(ctx, openfga.WriteAuthorizationModelRequest{
		SchemaVersion:   "1.1",
		TypeDefinitions: defs,
	})
}

func relationRef(objType, relation string) openfga.RelationReference {
	ref := openfga.RelationReference{Type: objType}
	if relation != "" {
		ref.Relation = &relation
	}
	return ref
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
