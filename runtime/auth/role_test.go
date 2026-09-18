package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoleHierarchy_Inherits_Exact(t *testing.T) {
	t.Parallel()
	h := RoleHierarchy{
		"viewer": {},
		"editor": {"viewer"},
		"admin":  {"editor", "viewer"},
	}
	assert.True(t, h.Inherits("admin", "admin"))
	assert.True(t, h.Inherits("viewer", "viewer"))
}

func TestRoleHierarchy_Inherits_Transitive(t *testing.T) {
	t.Parallel()
	h := RoleHierarchy{
		"viewer": {},
		"editor": {"viewer"},
		"admin":  {"editor", "viewer"},
	}
	assert.True(t, h.Inherits("admin", "viewer"))
	assert.True(t, h.Inherits("admin", "editor"))
	assert.True(t, h.Inherits("editor", "viewer"))
}

func TestRoleHierarchy_Inherits_Not(t *testing.T) {
	t.Parallel()
	h := RoleHierarchy{
		"viewer": {},
		"editor": {"viewer"},
		"admin":  {"editor", "viewer"},
	}
	assert.False(t, h.Inherits("viewer", "admin"))
	assert.False(t, h.Inherits("viewer", "editor"))
	assert.False(t, h.Inherits("editor", "admin"))
}

func TestRoleHierarchy_Inherits_Unknown(t *testing.T) {
	t.Parallel()
	h := RoleHierarchy{
		"viewer": {},
	}
	assert.False(t, h.Inherits("superadmin", "viewer"))
}

func TestRoleHierarchy_Inherits_Empty(t *testing.T) {
	t.Parallel()
	h := RoleHierarchy{}
	assert.False(t, h.Inherits("any", "other"))
}

func TestRoleHierarchy_Inherits_CycleSafe(t *testing.T) {
	t.Parallel()
	h := RoleHierarchy{
		"a": {"b"},
		"b": {"a"},
	}
	assert.False(t, h.Inherits("a", "missing"))
	assert.True(t, h.Inherits("a", "b"))
}

func TestRoleHierarchy_Satisfies(t *testing.T) {
	t.Parallel()
	h := RoleHierarchy{
		"viewer": {},
		"editor": {"viewer"},
		"admin":  {"editor", "viewer"},
	}
	assert.True(t, h.Satisfies([]string{"admin"}, "viewer"))
	assert.True(t, h.Satisfies([]string{"viewer", "editor"}, "viewer"))
	assert.False(t, h.Satisfies([]string{"viewer"}, "admin"))
	assert.False(t, h.Satisfies(nil, "viewer"))
}

func TestMatchPermission(t *testing.T) {
	t.Parallel()
	cases := []struct {
		granted, required string
		want              bool
	}{
		{"products:read", "products:read", true},
		{"products:*", "products:create", true},
		{"products:*", "products:edit:price", true},
		{"products:*", "admin:users:read", false},
		{"*", "anything:at:all", true},
		{"teams:members:*", "teams:members:invite", true},
		{"products:edit:name,price", "products:edit:price", false},
		{"", "products:read", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, MatchPermission(c.granted, c.required), "%s vs %s", c.granted, c.required)
	}
}

func TestMatchAnyPermission(t *testing.T) {
	t.Parallel()
	assert.True(t, MatchAnyPermission([]string{"admin:users:read", "products:*"}, "products:create"))
	assert.False(t, MatchAnyPermission([]string{"admin:users:read"}, "products:create"))
	assert.False(t, MatchAnyPermission(nil, "products:create"))
}
