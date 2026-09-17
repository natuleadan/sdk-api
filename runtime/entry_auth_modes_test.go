package runtime

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/natuleadan/sdk-api/server/middleware"
)

func basicTestValidator(_ context.Context, user, pass string) (*middleware.AuthContext, error) {
	if user == "u" && pass == "p" {
		return &middleware.AuthContext{UserID: "u", Roles: []string{"r"}}, nil
	}
	return nil, nil
}

func TestRegisterOneEntry_BasicMode(t *testing.T) {
	app := fiber.New()
	entry := &EntryDef{
		Type: "rest", Path: "/basic-test", Method: "GET", Handler: "basicHandler",
		AuthModes: []string{"basic"},
	}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{
		"basicHandler": func(c fiber.Ctx) error { return c.SendString("ok") },
	}}
	extra := entryAuthExtra{basicValidator: basicTestValidator}
	err := registerOneEntry(app, entry, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil, "none", nil, nil, "", 0, nil, nil, extra)
	if err != nil {
		t.Fatalf("registerOneEntry failed: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/basic-test", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	req2 := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/basic-test", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without credentials, got %d", resp2.StatusCode)
	}
}

func TestRegisterOneEntry_OAuthMode(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{"active":true,"sub":"oauth-user","scope":"read"}`))
	}))
	defer srv.Close()
	_ = hits

	app := fiber.New()
	entry := &EntryDef{
		Type: "rest", Path: "/oauth-test", Method: "GET", Handler: "oauthHandler",
		AuthModes: []string{"oauth"},
	}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{
		"oauthHandler": func(c fiber.Ctx) error { return c.SendString("ok") },
	}}
	extra := entryAuthExtra{oauth: &middleware.OAuthConfig{IntrospectionURL: srv.URL, CacheTTL: time.Minute}}
	err := registerOneEntry(app, entry, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil, "none", nil, nil, "", 0, nil, nil, extra)
	if err != nil {
		t.Fatalf("registerOneEntry failed: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/oauth-test", nil)
	req.Header.Set("Authorization", "Bearer tok-1")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRegisterOneEntry_SessionMode(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	store := redis.New(mr.Addr())

	ctx := context.Background()
	sid, err := middleware.CreateSession(ctx, store, time.Hour, "sess-user", []string{"r"})
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	entry := &EntryDef{
		Type: "rest", Path: "/sess-test", Method: "GET", Handler: "sessHandler",
		AuthModes: []string{"session"},
	}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{
		"sessHandler": func(c fiber.Ctx) error { return c.SendString("ok") },
	}}
	extra := entryAuthExtra{session: &middleware.SessionConfig{Cookie: "sid", Store: store, TTL: time.Hour}}
	err = registerOneEntry(app, entry, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil, "none", nil, nil, "", 0, nil, nil, extra)
	if err != nil {
		t.Fatalf("registerOneEntry failed: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/sess-test", nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: sid})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	req2 := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/sess-test", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without cookie, got %d", resp2.StatusCode)
	}
}

func TestValidateEntryAuthConfig_NewModes(t *testing.T) {
	newSvc := func() *Service {
		return &Service{config: &ServiceConfig{Auth: &AuthConfig{Driver: "none"}}}
	}
	cases := []struct {
		name    string
		mutate  func(svc *Service, entry *EntryDef)
		wantErr bool
	}{
		{"basic without validator", func(svc *Service, e *EntryDef) {
			e.AuthModes = []string{"basic"}
		}, true},
		{"basic with validator", func(svc *Service, e *EntryDef) {
			e.AuthModes = []string{"basic"}
			svc.basicValidator = basicTestValidator
		}, false},
		{"oauth without url", func(svc *Service, e *EntryDef) {
			e.AuthModes = []string{"oauth"}
		}, true},
		{"oauth with url", func(svc *Service, e *EntryDef) {
			e.AuthModes = []string{"oauth"}
			svc.config.Auth.OAuth = &OAuthConf{IntrospectionURL: "https://auth.example.com/introspect"}
		}, false},
		{"session without store", func(svc *Service, e *EntryDef) {
			e.AuthModes = []string{"session"}
			svc.config.Auth.Session = &SessionConf{}
		}, true},
		{"session with store", func(svc *Service, e *EntryDef) {
			e.AuthModes = []string{"session"}
			svc.config.Auth.Session = &SessionConf{Store: "cache-main"}
		}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc := newSvc()
			entry := &EntryDef{Type: "rest", Path: "/x"}
			tt.mutate(svc, entry)
			err := svc.validateEntryAuthConfig(entry, svc.config.Auth.Driver)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestRegisterOneEntry_JWTBasicCombo(t *testing.T) {
	app := fiber.New()
	entry := &EntryDef{
		Type: "rest", Path: "/combo-test", Method: "GET", Handler: "comboHandler",
		AuthModes: []string{"jwt", "basic"},
	}
	handlers := &EntryHandlers{Rest: map[string]func(fiber.Ctx) error{
		"comboHandler": func(c fiber.Ctx) error { return c.SendString("ok") },
	}}
	jwtCfg := &middleware.JWTConfig{Secret: "test", TokenLookup: "header:Authorization"}
	extra := entryAuthExtra{basicValidator: basicTestValidator}
	allowAll := func(context.Context, *middleware.AuthContext, []string, []string) error { return nil }
	err := registerOneEntry(app, entry, handlers, "/api/v1", nil, nil, jwtCfg, allowAll, nil, nil, nil, nil, "manual", nil, nil, "", 0, nil, nil, extra)
	if err != nil {
		t.Fatalf("registerOneEntry failed: %v", err)
	}

	basicReq := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/combo-test", nil)
	basicReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")))
	resp, err := app.Test(basicReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("basic cred on jwt+basic entry: expected 200, got %d", resp.StatusCode)
	}

	anonReq := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/combo-test", nil)
	resp2, err := app.Test(anonReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("no cred on jwt+basic entry: expected 401, got %d", resp2.StatusCode)
	}
}
