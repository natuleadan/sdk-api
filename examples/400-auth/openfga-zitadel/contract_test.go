package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/natuleadan/sdk-api/runtime/authtest"
	"github.com/natuleadan/sdk-api/server/auth/openfga"
)

// fgaSubjects implements authtest.SubjectProvider for the openfga-zitadel
// stack: identity is a Zitadel machine token (one subject) and authorization
// is OpenFGA. Because there is a single subject, Principal replaces any
// previously granted role with the requested one.
type fgaSubjects struct {
	subject string // FGA subject form: "user:<zitadel-user-id>"
	userID  string // raw Zitadel user id, as the API's AuthContext.UserID
	client  *openfga.Client
}

var knownRoles = []string{"viewer", "editor", "admin", "facturacion-lectura"}

func (p *fgaSubjects) ensure(t *testing.T) {
	t.Helper()
	if p.client != nil {
		return
	}
	_, subj := machineToken(t)
	p.subject = subj
	p.userID = strings.TrimPrefix(subj, "user:")
	p.client = fgaClient(t)
}

func (p *fgaSubjects) setRole(t *testing.T, role string) {
	t.Helper()
	p.ensure(t)
	for _, r := range knownRoles {
		_ = p.client.DeleteTuple(context.Background(), p.subject, "member", "role:"+r)
	}
	if role != "" {
		if err := p.client.AssignRole(context.Background(), p.subject, role); err != nil {
			t.Fatalf("assign %s: %v", role, err)
		}
	}
}

// Principal returns a fresh subject with exactly the requested role. The stack
// is single-subject (one machine token) and the token carries no org claim, so
// it implements neither TenantSubjectProvider nor CookieSubjectProvider; the
// contract skips those sections with a log line rather than faking an org.
func (p *fgaSubjects) Principal(t *testing.T, roles ...string) authtest.Subject {
	t.Helper()
	role := "viewer"
	if len(roles) > 0 {
		role = roles[0]
	}
	p.setRole(t, role)
	token, _ := machineToken(t)
	return authtest.Subject{Token: token, ID: p.userID}
}

func (p *fgaSubjects) Revoke(t *testing.T, _ authtest.Subject, _ string) {
	t.Helper()
	p.setRole(t, "")
}

func (p *fgaSubjects) APIKey(t *testing.T, name string) (string, string) {
	t.Helper()
	switch name {
	case "reader":
		return "sk-reader_abc123", "viewer"
	case "editor":
		return "sk-editor_abc123", "editor"
	default:
		return "", ""
	}
}

// TestAuthContract runs the shared auth contract for driver "openfga-zitadel".
func TestAuthContract(t *testing.T) {
	if os.Getenv("DOCKER_TEST") != "1" {
		t.Skip("Docker-only test")
	}
	authtest.Run(t, authtest.Config{BaseURL: baseURL}, &fgaSubjects{})
}
