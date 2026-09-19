package main

import (
	"net/http"
	"os"
	"testing"

	"github.com/natuleadan/sdk-api/runtime/authtest"
)

// manualSubjects implements authtest.SubjectProvider over the manual driver:
// identity is a username/password login and authorization is role-based. The
// viewer subject is a non-seeded user (user-contract) whose roles are assigned
// dynamically, so the contract can both grant and fully revoke them.
type manualSubjects struct{}

const (
	contractRoleUser   = "user-contract"
	contractRoleLogin  = "contract-role"
	contractRolePass   = "pass123"
	manualAdminLogin   = "rolesadmin"
	manualAdminPass    = "pass123"
	legacyEditorUser   = "user-contract-editor"
	legacyEditorLogin  = "contract-editor"
	legacyEditorPasswd = "pass123"
)

func (manualSubjects) Principal(t *testing.T, roles ...string) authtest.Subject {
	t.Helper()
	role := "viewer"
	if len(roles) > 0 {
		role = roles[0]
	}
	switch role {
	case "admin":
		tok := loginRetry(t, manualAdminLogin, manualAdminPass)
		return authtest.Subject{Token: tok, ID: "user-roles-admin"}
	case "editor":
		tok := loginRetry(t, "rolesuser", seedPass)
		return authtest.Subject{Token: tok, ID: "user-roles"}
	default: // viewer, readonly
		// Ensure the dynamically-assigned viewer role is present.
		adminTok := loginRetry(t, manualAdminLogin, manualAdminPass)
		_, _ = rolesReq(t, http.MethodPost, "/admin/users/"+contractRoleUser+"/roles", adminTok,
			map[string]any{"role": "viewer"})
		tok := loginRetry(t, contractRoleLogin, contractRolePass)
		return authtest.Subject{Token: tok, ID: contractRoleUser}
	}
}

func (manualSubjects) Revoke(t *testing.T, s authtest.Subject, role string) {
	t.Helper()
	adminTok := loginRetry(t, manualAdminLogin, manualAdminPass)
	_, _ = rolesReq(t, http.MethodDelete, "/admin/users/"+s.ID+"/roles/"+role, adminTok, map[string]any{})
	// Also clear the legacy users.role so a downgraded user is truly denied.
	if role == "viewer" {
		_, _ = rolesReq(t, http.MethodPatch, "/admin/users/"+s.ID+"/role", adminTok, map[string]any{"role": ""})
	}
}

func (manualSubjects) APIKey(t *testing.T, name string) (string, string) {
	t.Helper()
	switch name {
	case "reader":
		return "sk-viewer_abc123", "viewer"
	case "editor":
		return "sk-editor_abc123", "editor"
	default:
		return "", ""
	}
}

// PrincipalWithTenant maps orgs to seed users: viewer lives in org-beta, every
// other role lands in org-alfa (see auth_login.go). Tenant entries carry no
// role restriction, so any authenticated user of the org works. The org-alfa
// side uses rolesadmin (not admin) because the account-lockout tests lock the
// "admin" username for the rest of the run.
func (manualSubjects) PrincipalWithTenant(t *testing.T, tenant string, roles ...string) authtest.Subject {
	t.Helper()
	if tenant == "org-beta" {
		return authtest.Subject{Token: loginRetry(t, "viewer", seedPass), ID: "user-viewer"}
	}
	return authtest.Subject{Token: loginRetry(t, manualAdminLogin, manualAdminPass), ID: "user-roles-admin"}
}

// PrincipalInOrg is the multi-org variant used for cross-tenant isolation.
func (manualSubjects) PrincipalInOrg(t *testing.T, org string, roles ...string) authtest.Subject {
	t.Helper()
	return manualSubjects{}.PrincipalWithTenant(t, org, roles...)
}

// Cookie returns the encrypted login cookie issued by POST /login (via a
// user the lockout tests never touch).
func (manualSubjects) Cookie(t *testing.T) (string, string) {
	t.Helper()
	_, enc := loginAndCookie(t, manualAdminLogin, manualAdminPass)
	return "token", enc
}

// TestAuthContract runs the shared auth contract for driver "manual".
func TestAuthContract(t *testing.T) {
	if os.Getenv("DOCKER_TEST") != "1" {
		t.Skip("Docker-only test")
	}
	authtest.Run(t, authtest.Config{BaseURL: baseURL}, manualSubjects{})
}
