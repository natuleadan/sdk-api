// Package ory provides Ory Kratos authentication and Keto authorization client.
package ory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/natuleadan/sdk-api/server/auth/jwks"
)

// Client wraps Ory Kratos (auth) and Keto (authorization).
type Client struct {
	kratosPublicURL string
	ketoReadURL     string
	ketoWriteURL    string
	http            *http.Client
	resolver        *jwks.Resolver

	roleNamespace      string
	roleRelation       string
	permissionRelation string
}

// Config holds Ory connection settings.
type Config struct {
	KratosPublicURL string
	// KetoURL is the base Keto URL, used for both reads (checks) and writes
	// when the specific URLs below are empty.
	KetoURL string
	// KetoReadURL is the Keto read API (checks); defaults to KetoURL.
	KetoReadURL string
	// KetoWriteURL is the Keto write API (tuples); defaults to KetoURL.
	KetoWriteURL string
	TTL          time.Duration
	// RoleNamespace/RoleRelation name the Keto tuple that grants a role
	// (default: roles / assignee). PermissionRelation names the relation that
	// grants a permission on a resource namespace (default: perform).
	RoleNamespace      string
	RoleRelation       string
	PermissionRelation string
}

// NewClient creates an Ory client (Kratos + Keto).
func NewClient(cfg Config) *Client {
	var opts []jwks.Option
	if cfg.TTL > 0 {
		opts = append(opts, jwks.WithTTL(cfg.TTL))
	}
	c := &Client{
		kratosPublicURL: cfg.KratosPublicURL,
		ketoReadURL:     firstNonEmpty(cfg.KetoReadURL, cfg.KetoURL),
		ketoWriteURL:    firstNonEmpty(cfg.KetoWriteURL, cfg.KetoURL),
		http:            &http.Client{Timeout: 10 * time.Second},
		resolver:        jwks.New(cfg.KratosPublicURL+"/.well-known/jwks.json", opts...),

		roleNamespace:      cfg.RoleNamespace,
		roleRelation:       cfg.RoleRelation,
		permissionRelation: cfg.PermissionRelation,
	}
	if c.roleNamespace == "" {
		c.roleNamespace = "roles"
	}
	if c.roleRelation == "" {
		c.roleRelation = "assignee"
	}
	if c.permissionRelation == "" {
		c.permissionRelation = "perform"
	}
	return c
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// RoleNamespace returns the Keto namespace that models role membership.
func (c *Client) RoleNamespace() string { return c.roleNamespace }

// RoleRelation returns the Keto relation that grants a role to a subject.
func (c *Client) RoleRelation() string { return c.roleRelation }

// PermissionRelation returns the Keto relation that grants a permission.
func (c *Client) PermissionRelation() string { return c.permissionRelation }

// Session holds the validated user session from Kratos.
type Session struct {
	Identity struct {
		ID     string         `json:"id"`
		Traits map[string]any `json:"traits"`
	} `json:"identity"`
}

// OrgID extracts the tenant/organization from the identity traits ("org_id").
// Returns "" when absent.
func (s *Session) OrgID() string {
	if s == nil || s.Identity.Traits == nil {
		return ""
	}
	if v, ok := s.Identity.Traits["org_id"].(string); ok {
		return v
	}
	return ""
}

// Roles extracts role names from the identity traits ("roles" as a list of
// strings). Keto remains the authorization source of truth; this is only a
// convenience for drivers that mirror roles into traits.
func (s *Session) Roles() []string {
	if s == nil || s.Identity.Traits == nil {
		return nil
	}
	raw, ok := s.Identity.Traits["roles"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		roles := make([]string, 0, len(v))
		for _, item := range v {
			if r, ok := item.(string); ok && r != "" {
				roles = append(roles, r)
			}
		}
		return roles
	default:
		return nil
	}
}

// ValidateSession validates a session cookie or token against Ory Kratos.
func (c *Client) ValidateSession(ctx context.Context, token string) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.kratosPublicURL+"/sessions/whoami", nil)
	if err != nil {
		return nil, fmt.Errorf("ory: failed to create whoami request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ory: whoami request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "ory: whoami body close error: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ory: whoami returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ory: failed to read whoami response: %w", err)
	}

	var session Session
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("ory: failed to parse whoami response: %w", err)
	}

	return &session, nil
}

// ValidateJWT validates a JWT signed by Ory Kratos using JWKS.
func (c *Client) ValidateJWT(_ context.Context, tokenString string, _ string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
	)

	token, err := parser.Parse(tokenString, c.resolver.KeyFunc())
	if err != nil {
		return nil, fmt.Errorf("ory: jwt validation failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("ory: invalid token claims")
	}

	return claims, nil
}

// KetoCheckRequest defines an authorization check against Ory Keto.
type KetoCheckRequest struct {
	Namespace string
	Object    string
	Relation  string
	SubjectID string
}

// KetoCheck performs an authorization check against Ory Keto.
func (c *Client) KetoCheck(ctx context.Context, req KetoCheckRequest) (bool, error) {
	body, err := json.Marshal(map[string]any{
		"namespace":  req.Namespace,
		"object":     req.Object,
		"relation":   req.Relation,
		"subject_id": req.SubjectID,
	})
	if err != nil {
		return false, fmt.Errorf("ory: keto check marshal failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.ketoReadURL+"/relation-tuples/check", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("ory: keto check request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("ory: keto check failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "ory: keto body close error: %v\n", err)
		}
	}()

	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("ory: failed to parse keto check response: %w", err)
	}

	return result.Allowed, nil
}

// WriteKetoTuple writes a relation tuple to Ory Keto.
func (c *Client) WriteKetoTuple(ctx context.Context, namespace, object, relation, subjectID string) error {
	body, err := json.Marshal(map[string]any{
		"namespace":  namespace,
		"object":     object,
		"relation":   relation,
		"subject_id": subjectID,
	})
	if err != nil {
		return fmt.Errorf("ory: keto write marshal failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.ketoWriteURL+"/admin/relation-tuples", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ory: keto write request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ory: keto write tuple failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "ory: keto write close error: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ory: keto write tuple returned %d", resp.StatusCode)
	}

	return nil
}

// PermissionDef declares a role → permission grant seeded into Keto.
type PermissionDef struct {
	Role     string
	Resource string
	Action   string
}

// SeedPermissions writes the role → permission tuples that let a role reach a
// permission through a Keto subject set:
//
//	<resource>:<action>#<permission_relation>@<role_namespace>:<role>#<role_relation>
//
// Idempotent: Keto's write API returns 201 on create and 409/200 on repeat.
func (c *Client) SeedPermissions(ctx context.Context, permissions []PermissionDef) error {
	for _, p := range permissions {
		if p.Role == "" || p.Resource == "" || p.Action == "" {
			continue
		}
		body, err := json.Marshal(map[string]any{
			"namespace": p.Resource,
			"object":    p.Action,
			"relation":  c.permissionRelation,
			"subject_set": map[string]any{
				"namespace": c.roleNamespace,
				"object":    p.Role,
				"relation":  c.roleRelation,
			},
		})
		if err != nil {
			return fmt.Errorf("ory: keto seed marshal failed: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
			c.ketoWriteURL+"/admin/relation-tuples", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("ory: keto seed request failed: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(httpReq)
		if err != nil {
			return fmt.Errorf("ory: keto seed failed: %w", err)
		}
		// 201 created, 200/409 when the tuple already exists: all are fine.
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
			_ = resp.Body.Close()
			return fmt.Errorf("ory: keto seed returned %d", resp.StatusCode)
		}
		if err := resp.Body.Close(); err != nil {
			return fmt.Errorf("ory: keto seed close: %w", err)
		}
	}
	return nil
}
