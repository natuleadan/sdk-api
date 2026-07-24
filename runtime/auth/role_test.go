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
