package main

import (
	"net/http"
	"testing"
)

// grantReq is rolesReq with a JSON body already provided (grants/teams).
func grantReq(t *testing.T, method, path, token string, payload any) (int, string) {
	t.Helper()
	return rolesReq(t, method, path, token, payload)
}

// TestGrants_DelegateReadViaPermission grants products:read to a viewer,
// proving a delegated grant unlocks a permission the role lacks.
func TestGrants_DelegateReadViaPermission(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	viewerTok := loginRetry(t, "viewer", seedPass)
	revokeAllGrants(t, adminTok, "user-viewer")

	// Admin holds products:read and can delegate it.
	code, body := grantReq(t, http.MethodPost, "/auth/grants", adminTok, map[string]any{
		"grantee": "user-viewer", "permission": "products:read",
	})
	if code != 200 {
		t.Fatalf("grant = %d, want 200 (%s)", code, body)
	}
	// The viewer role has no products:read, so this must be the delegated grant.
	if code, body := grantReq(t, http.MethodGet, "/debug/items", viewerTok, nil); code != 200 {
		t.Fatalf("viewer /debug/items after grant = %d, want 200 (%s)", code, body)
	}
}

// TestGrants_NoEscalation proves a grantor cannot delegate beyond their set.
func TestGrants_NoEscalation(t *testing.T) {
	viewerTok := loginRetry(t, "viewer", seedPass)
	// viewer does not hold users:manage, so it cannot grant it.
	code, _ := grantReq(t, http.MethodPost, "/auth/grants", viewerTok, map[string]any{
		"grantee": "user-editor", "permission": "users:manage",
	})
	if code != 403 {
		t.Fatalf("viewer granting users:manage = %d, want 403", code)
	}
}

// TestGrants_Revoke removes a delegated grant and checks it stops working.
func TestGrants_Revoke(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	editorTok := loginRetry(t, "rolesuser", seedPass)
	revokeAllGrants(t, adminTok, "user-roles")

	code, body := grantReq(t, http.MethodPost, "/auth/grants", adminTok, map[string]any{
		"grantee": "user-roles", "permission": "products:read",
	})
	if code != 200 {
		t.Fatalf("grant = %d, want 200 (%s)", code, body)
	}
	id := jsonField(t, body, "id")
	if id == "" {
		t.Fatalf("no grant id in %s", body)
	}
	if code, _ := grantReq(t, http.MethodDelete, "/auth/grants/"+id, adminTok, map[string]any{}); code != 200 {
		t.Fatalf("revoke = %d, want 200", code)
	}
	if code, body := grantReq(t, http.MethodGet, "/debug/items", editorTok, nil); code != 403 {
		t.Fatalf("editor /debug/items after revoke = %d, want 403 (%s)", code, body)
	}
}

// revokeAllGrants removes every active grant owned by grantor for grantee by
// querying the listing and deleting each matching id.
func revokeAllGrants(t *testing.T, grantorTok, grantee string) {
	t.Helper()
	for range 50 {
		_, listBody := grantReq(t, http.MethodGet, "/auth/grants", grantorTok, nil)
		i := indexOf(listBody, `"Grantee":"`+grantee+`"`)
		if i < 0 {
			return
		}
		head := listBody[:i]
		j := lastIndexOf(head, `"ID":"`)
		if j < 0 {
			return
		}
		rest := head[j+len(`"ID":"`):]
		k := indexOf(rest, `"`)
		if k < 0 {
			return
		}
		if code, _ := grantReq(t, http.MethodDelete, "/auth/grants/"+rest[:k], grantorTok, map[string]any{}); code != 200 {
			return
		}
	}
}

func lastIndexOf(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestTeams_CreateAndList exercises the teams endpoints.
func TestTeams_CreateAndList(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)

	code, body := grantReq(t, http.MethodPost, "/teams", adminTok, map[string]any{"name": "Equipo QA"})
	if code != 201 {
		t.Fatalf("create team = %d, want 201 (%s)", code, body)
	}
	code, listBody := grantReq(t, http.MethodGet, "/teams", adminTok, nil)
	if code != 200 {
		t.Fatalf("list teams = %d, want 200 (%s)", code, listBody)
	}
	if !contains(listBody, "Equipo QA") {
		t.Fatalf("created team missing from listing: %s", listBody)
	}
}

// TestGrants_NotAllowedByDefault proves a permission-less user is denied.
// Uses the editor identity which holds no products:read grant by default.
func TestGrants_NotAllowedByDefault(t *testing.T) {
	adminTok := loginRetry(t, "rolesadmin", seedPass)
	editorTok := loginRetry(t, "rolesuser", seedPass)
	revokeAllGrants(t, adminTok, "user-roles")
	if code, _ := grantReq(t, http.MethodGet, "/debug/items", editorTok, nil); code != 403 {
		t.Fatalf("editor /debug/items = %d, want 403", code)
	}
}

// jsonField extracts the first string value of a JSON field (rough, enough for
// a single id). Avoids importing a full parser for one value.
func jsonField(t *testing.T, body, field string) string {
	t.Helper()
	key := `"` + field + `":"`
	i := indexOf(body, key)
	if i < 0 {
		return ""
	}
	rest := body[i+len(key):]
	j := indexOf(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func contains(s, sub string) bool { return indexOf(s, sub) >= 0 }
