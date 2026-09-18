//go:build integration

package openfga

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

const openfgaAPIURL = "http://localhost:18080"

func skipIfNoOpenFGA(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", "localhost:18081")
	if err != nil {
		t.Skipf("OpenFGA not available at localhost:18081: %v", err)
	}
	conn.Close()
}

func ensureOpenFGAStore(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, openfgaAPIURL+"/stores", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("list stores failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var list struct {
		Stores []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"stores"`
	}
	json.Unmarshal(body, &list)

	for _, s := range list.Stores {
		if s.Name == "sdk-api-test" {
			return s.ID
		}
	}

	createBody := `{"name":"sdk-api-test"}`
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, openfgaAPIURL+"/stores", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &created)
	if created.ID == "" {
		t.Fatalf("failed to create OpenFGA store: %s", string(body))
	}
	return created.ID
}

func writeTestModel(t *testing.T, storeID string) {
	t.Helper()
	body := `{
		"schema_version":"1.1",
		"type_definitions":[
			{"type":"user"},
			{
				"type":"document",
				"relations":{
					"can_read":{"this":{}},
					"can_write":{"this":{}}
				},
				"metadata":{
					"relations":{
						"can_read":{"directly_related_user_types":[{"type":"user"}]},
						"can_write":{"directly_related_user_types":[{"type":"user"}]}
					}
				}
			}
		]
	}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		openfgaAPIURL+"/stores/"+storeID+"/authorization-models",
		bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("write model request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("write model request do failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode > 299 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("write model returned %d: %s", resp.StatusCode, string(respBody))
	}
}

func TestIntegration_OpenFGA_Check(t *testing.T) {
	t.Parallel()
	skipIfNoOpenFGA(t)
	ctx := context.Background()
	storeID := ensureOpenFGAStore(t)
	writeTestModel(t, storeID)

	client, err := NewClient(Config{APIURL: openfgaAPIURL, StoreID: storeID})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	err = client.WriteTuple(ctx, "user:alice", "can_read", "document:test")
	if err != nil {
		t.Fatalf("WriteTuple failed: %v", err)
	}
	t.Cleanup(func() { client.DeleteTuple(ctx, "user:alice", "can_read", "document:test") })

	allowed, err := client.Check(ctx, CheckRequest{
		User: "user:alice", Relation: "can_read", Object: "document:test",
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !allowed {
		t.Error("expected allowed=true for user:alice can_read document:test")
	}

	denied, err := client.Check(ctx, CheckRequest{
		User: "user:bob", Relation: "can_read", Object: "document:test",
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if denied {
		t.Error("expected allowed=false for user:bob")
	}
}

func TestIntegration_OpenFGA_DeleteTuple(t *testing.T) {
	t.Parallel()
	skipIfNoOpenFGA(t)
	ctx := context.Background()
	storeID := ensureOpenFGAStore(t)
	writeTestModel(t, storeID)

	client, err := NewClient(Config{APIURL: openfgaAPIURL, StoreID: storeID})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	err = client.WriteTuple(ctx, "user:temp", "can_write", "document:temp")
	if err != nil {
		t.Fatalf("WriteTuple failed: %v", err)
	}

	err = client.DeleteTuple(ctx, "user:temp", "can_write", "document:temp")
	if err != nil {
		t.Fatalf("DeleteTuple failed: %v", err)
	}

	allowed, err := client.Check(ctx, CheckRequest{
		User: "user:temp", Relation: "can_write", Object: "document:temp",
	})
	if err != nil {
		t.Fatalf("Check after delete failed: %v", err)
	}
	if allowed {
		t.Error("expected allowed=false after tuple deletion")
	}
}

func TestIntegration_OpenFGA_CheckRoleAssignment(t *testing.T) {
	t.Parallel()
	skipIfNoOpenFGA(t)
	ctx := context.Background()
	storeID := ensureOpenFGAStore(t)
	writeTestModel(t, storeID)

	client, err := NewClient(Config{APIURL: openfgaAPIURL, StoreID: storeID})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Use only types defined in the model (user, document)
	err = client.WriteTuple(ctx, "user:admin", "can_read", "document:admin-doc")
	if err != nil {
		t.Fatalf("WriteTuple failed: %v", err)
	}
	t.Cleanup(func() { client.DeleteTuple(ctx, "user:admin", "can_read", "document:admin-doc") })

	allowed, err := client.Check(ctx, CheckRequest{
		User: "user:admin", Relation: "can_read", Object: "document:admin-doc",
	})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !allowed {
		t.Error("expected allowed=true for user:admin")
	}
}

func ensureOpenFGAStoreNamed(t *testing.T, name string) string {
	t.Helper()
	ctx := context.Background()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, openfgaAPIURL+"/stores", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("list stores failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var list struct {
		Stores []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"stores"`
	}
	json.Unmarshal(body, &list)
	for _, s := range list.Stores {
		if s.Name == name {
			return s.ID
		}
	}

	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, openfgaAPIURL+"/stores",
		strings.NewReader(`{"name":"`+name+`"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatalf("create store failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body, &created)
	if created.ID == "" {
		t.Fatalf("failed to create store: %s", string(body))
	}
	return created.ID
}

// TestIntegration_OpenFGA_RolesAndPermissions covers the SDK convention
// end-to-end: a user that is `member` of role:<name> inherits can_<action>.
func TestIntegration_OpenFGA_RolesAndPermissions(t *testing.T) {
	skipIfNoOpenFGA(t)
	ctx := context.Background()
	storeID := ensureOpenFGAStoreNamed(t, "sdk-api-test-role")
	client, err := NewClient(Config{APIURL: openfgaAPIURL, StoreID: storeID})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	permissions := []PermissionDef{
		{Role: "admin", Resource: "products", Actions: []string{"create"}},
		{Role: "admin", Resource: "users", Actions: []string{"manage"}},
		{Role: "viewer", Resource: "products", Actions: []string{"read"}},
	}
	if _, err := client.EnsureModel(ctx, permissions); err != nil {
		t.Fatalf("EnsureModel failed: %v", err)
	}
	if err := client.SeedPermissions(ctx, permissions); err != nil {
		t.Fatalf("SeedPermissions failed: %v", err)
	}
	if err := client.AssignRole(ctx, "user:admin", "admin"); err != nil {
		t.Fatalf("AssignRole failed: %v", err)
	}

	checks := []struct {
		desc string
		req  CheckRequest
		want bool
	}{
		{"admin is member of role:admin", CheckRequest{User: "user:admin", Relation: "member", Object: "role:admin"}, true},
		{"admin inherits can_manage on users:manage", CheckRequest{User: "user:admin", Relation: "can_manage", Object: "users:manage"}, true},
		{"admin inherits can_create on products:create", CheckRequest{User: "user:admin", Relation: "can_create", Object: "products:create"}, true},
		{"admin lacks can_read (not granted)", CheckRequest{User: "user:admin", Relation: "can_read", Object: "products:read"}, false},
		{"bob is not a member", CheckRequest{User: "user:bob", Relation: "member", Object: "role:admin"}, false},
		{"bob has no permission", CheckRequest{User: "user:bob", Relation: "can_manage", Object: "users:manage"}, false},
	}
	for _, c := range checks {
		got, err := client.Check(ctx, c.req)
		if err != nil {
			t.Fatalf("%s: check failed: %v", c.desc, err)
		}
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.desc, got, c.want)
		}
	}
}

// TestIntegration_OpenFGA_RolesWithoutPermissions covers arbitrary role names
// with no declared permissions: the base model must still allow membership.
func TestIntegration_OpenFGA_RolesWithoutPermissions(t *testing.T) {
	skipIfNoOpenFGA(t)
	ctx := context.Background()
	storeID := ensureOpenFGAStoreNamed(t, "sdk-api-test-role-only")
	client, err := NewClient(Config{APIURL: openfgaAPIURL, StoreID: storeID})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if _, err := client.EnsureModel(ctx, nil); err != nil {
		t.Fatalf("EnsureModel failed: %v", err)
	}

	// Arbitrary, unrelated role names must work.
	for _, role := range []string{"support_tier_2", "facturacion-lectura", "ROLE_42"} {
		subject := "user:" + role
		if err := client.AssignRole(ctx, subject, role); err != nil {
			t.Fatalf("AssignRole %s failed: %v", role, err)
		}
		allowed, err := client.Check(ctx, CheckRequest{
			User: subject, Relation: "member", Object: "role:" + role,
		})
		if err != nil {
			t.Fatalf("check %s failed: %v", role, err)
		}
		if !allowed {
			t.Errorf("expected member for arbitrary role %q", role)
		}
	}
}
