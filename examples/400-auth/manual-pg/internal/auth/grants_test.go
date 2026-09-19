package auth

import (
	"testing"
	"time"

	sdkauth "github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/stretchr/testify/assert"
)

// memStore is an in-memory Store for unit tests.
type memStore struct {
	grants []Grant
	seq    int
}

func (m *memStore) InsertGrant(grantor, grantee, permission, resource string, expiresAt *time.Time) (string, error) {
	m.seq++
	id := "g" + string(rune('0'+m.seq))
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return id, nil
	}
	m.grants = append(m.grants, Grant{ID: id, Grantor: grantor, Grantee: grantee, Permission: permission, Resource: resource, Status: "active"})
	return id, nil
}

func (m *memStore) ActiveGrants(grantee string) ([]Grant, error) {
	var out []Grant
	for _, g := range m.grants {
		if g.Grantee == grantee && g.Status == "active" {
			out = append(out, g)
		}
	}
	return out, nil
}

func (m *memStore) RevokeGrant(id, grantor string) (bool, error) {
	for i := range m.grants {
		if m.grants[i].ID == id && m.grants[i].Grantor == grantor {
			m.grants[i].Status = "revoked"
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) ListGrants(user string) ([]Grant, error) {
	var out []Grant
	for _, g := range m.grants {
		if g.Grantor == user || g.Grantee == user {
			out = append(out, g)
		}
	}
	return out, nil
}

var (
	hierarchy = sdkauth.RoleHierarchy{
		"viewer":     {},
		"editor":     {"viewer"},
		"teamadmin":  {"editor", "viewer"},
		"admin":      {"teamadmin", "editor", "viewer"},
		"superadmin": {"admin", "teamadmin", "editor", "viewer"},
	}
	table = RolePermissions{
		"superadmin": {"products:*", "admin:*", "teams:*", "auth:*"},
		"admin":      {"products:*", "admin:users:read", "auth:grants:*"},
		"teamadmin":  {"products:create", "products:edit:*", "products:read", "auth:grants:grant"},
		"editor":     {"products:create", "products:edit:*", "products:read", "auth:grants:grant"},
		"viewer":     {"products:read"},
		"auditor":    {"admin:logs:read"},
	}
)

func TestAuthorize_RoleHierarchy(t *testing.T) {
	// admin inherits editor -> viewer; editor must NOT reach admin.
	assert.NoError(t, Authorize(hierarchy, table, nil, "u1", []string{"admin"}, []string{"editor"}, nil))
	assert.NoError(t, Authorize(hierarchy, table, nil, "u1", []string{"admin"}, []string{"viewer"}, nil))
	assert.Error(t, Authorize(hierarchy, table, nil, "u1", []string{"editor"}, []string{"admin"}, nil))
}

func TestAuthorize_WildcardPermission(t *testing.T) {
	assert.NoError(t, Authorize(hierarchy, table, nil, "u1", []string{"admin"}, nil, []string{"products:edit:price"}))
	assert.NoError(t, Authorize(hierarchy, table, nil, "u1", []string{"superadmin"}, nil, []string{"anything:at:all"}))
	// viewer only has products:read, not products:create
	assert.Error(t, Authorize(hierarchy, table, nil, "u1", []string{"viewer"}, nil, []string{"products:create"}))
}

func TestCanDelegate_NoEscalation(t *testing.T) {
	// editor may grant products:create (holds products:create)
	assert.True(t, CanDelegate([]string{"editor"}, table, "products:create"))
	// viewer may not grant products:create
	assert.False(t, CanDelegate([]string{"viewer"}, table, "products:create"))
	// auditor may not grant products:read
	assert.False(t, CanDelegate([]string{"auditor"}, table, "products:read"))
	// superadmin may grant anything
	assert.True(t, CanDelegate([]string{"superadmin"}, table, "admin:logs:read"))
}

func TestEffectivePermissions_WithGrants(t *testing.T) {
	store := &memStore{}
	// viewer has no products:create natively...
	if _, err := store.InsertGrant("admin-user", "viewer-user", "products:create", "*", nil); err != nil {
		t.Fatal(err)
	}
	err := Authorize(hierarchy, table, store, "viewer-user", []string{"viewer"}, nil, []string{"products:create"})
	assert.NoError(t, err, "delegated grant should allow the permission")

	// a different user is unaffected
	err = Authorize(hierarchy, table, store, "other-viewer", []string{"viewer"}, nil, []string{"products:create"})
	assert.Error(t, err)
}

func TestAuthorize_RevokedGrantDoesNotApply(t *testing.T) {
	store := &memStore{}
	id, _ := store.InsertGrant("admin-user", "viewer-user", "products:create", "*", nil)
	assert.NoError(t, Authorize(hierarchy, table, store, "viewer-user", []string{"viewer"}, nil, []string{"products:create"}))
	ok, err := store.RevokeGrant(id, "admin-user")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Error(t, Authorize(hierarchy, table, store, "viewer-user", []string{"viewer"}, nil, []string{"products:create"}))
}

func TestAuthorize_ExpiredGrantDoesNotApply(t *testing.T) {
	store := &memStore{}
	past := time.Now().Add(-time.Hour)
	_, _ = store.InsertGrant("admin-user", "viewer-user", "products:create", "*", &past)
	assert.Error(t, Authorize(hierarchy, table, store, "viewer-user", []string{"viewer"}, nil, []string{"products:create"}))
}

func TestAuthorize_NoRequirements(t *testing.T) {
	assert.NoError(t, Authorize(hierarchy, table, nil, "u1", nil, nil, nil))
}
