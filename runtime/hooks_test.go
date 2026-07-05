package runtime

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// testModel simulates a CRUD model
type testModel struct {
	ID   int64  `json:"id"   db:"id,primary,auto"`
	Name string `json:"name" db:"name,required"`
}

// --- Entry Hooks tests ---

type trackingHooks struct {
	DefaultHooks[testModel]
	created     bool
	createCalls int
	updated     bool
	updateCalls int
	deleted     bool
	deleteCalls int
}

func (h *trackingHooks) AfterCreate(_ context.Context, _ *testModel) error {
	h.created = true
	h.createCalls++
	return nil
}

func (h *trackingHooks) AfterUpdate(_ context.Context, _ *testModel) error {
	h.updated = true
	h.updateCalls++
	return nil
}

func (h *trackingHooks) AfterDelete(_ context.Context, _ string) error {
	h.deleted = true
	h.deleteCalls++
	return nil
}

type transformHooks struct {
	DefaultHooks[testModel]
	beforeCalled bool
	afterCalled  bool
	modifiedName string
}

func (h *transformHooks) BeforeTransform(_ context.Context, req testModel) (testModel, error) {
	h.beforeCalled = true
	if h.modifiedName != "" {
		req.Name = h.modifiedName
	}
	return req, nil
}

func (h *transformHooks) AfterTransform(_ context.Context, _ any) error {
	h.afterCalled = true
	return nil
}

type validationHooks struct {
	DefaultHooks[testModel]
	rejectCreate bool
	rejectDelete bool
}

func (h *validationHooks) BeforeCreate(_ context.Context, req testModel) (testModel, error) {
	if h.rejectCreate {
		return req, context.DeadlineExceeded
	}
	return req, nil
}

func (h *validationHooks) BeforeDelete(_ context.Context, _ string) error {
	if h.rejectDelete {
		return context.DeadlineExceeded
	}
	return nil
}

func TestDefaultHooks_NoOp(t *testing.T) {
	var h DefaultHooks[testModel]
	ctx := context.Background()

	// All methods should return nil/unchanged
	req := testModel{Name: "test"}
	out, err := h.BeforeCreate(ctx, req)
	if err != nil || out.Name != "test" {
		t.Errorf("BeforeCreate: %v, %v", err, out)
	}
	err = h.AfterCreate(ctx, &req)
	if err != nil {
		t.Errorf("AfterCreate: %v", err)
	}
	patch := map[string]any{"name": "x"}
	patch, err = h.BeforeUpdate(ctx, "1", patch)
	if err != nil || patch["name"] != "x" {
		t.Errorf("BeforeUpdate: %v, %v", err, patch)
	}
	err = h.AfterUpdate(ctx, &req)
	if err != nil {
		t.Errorf("AfterUpdate: %v", err)
	}
	err = h.BeforeDelete(ctx, "1")
	if err != nil {
		t.Errorf("BeforeDelete: %v", err)
	}
	err = h.AfterDelete(ctx, "1")
	if err != nil {
		t.Errorf("AfterDelete: %v", err)
	}
	out, err = h.BeforeTransform(ctx, req)
	if err != nil || out.Name != "test" {
		t.Errorf("BeforeTransform: %v, %v", err, out)
	}
	err = h.AfterTransform(ctx, req)
	if err != nil {
		t.Errorf("AfterTransform: %v", err)
	}
}

func TestEntryHooks_TrackingHooks(t *testing.T) {
	h := &trackingHooks{}
	ctx := context.Background()

	h.AfterCreate(ctx, &testModel{})
	if !h.created {
		t.Error("AfterCreate should set created=true")
	}
	if h.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", h.createCalls)
	}

	h.AfterUpdate(ctx, &testModel{})
	if !h.updated {
		t.Error("AfterUpdate should set updated=true")
	}

	h.AfterDelete(ctx, "1")
	if !h.deleted {
		t.Error("AfterDelete should set deleted=true")
	}
}

func TestEntryHooks_TransformHooks(t *testing.T) {
	h := &transformHooks{modifiedName: "transformed"}
	ctx := context.Background()

	req := testModel{Name: "original"}
	out, err := h.BeforeTransform(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !h.beforeCalled {
		t.Error("BeforeTransform should be called")
	}
	if out.Name != "transformed" {
		t.Errorf("Name = %q, want transformed", out.Name)
	}

	h.AfterTransform(ctx, "result")
	if !h.afterCalled {
		t.Error("AfterTransform should be called")
	}
}

func TestEntryHooks_ValidationHooks(t *testing.T) {
	ctx := context.Background()

	t.Run("reject create", func(t *testing.T) {
		h := &validationHooks{rejectCreate: true}
		_, err := h.BeforeCreate(ctx, testModel{})
		if err == nil {
			t.Error("expected error from BeforeCreate")
		}
	})

	t.Run("allow create", func(t *testing.T) {
		h := &validationHooks{rejectCreate: false}
		_, err := h.BeforeCreate(ctx, testModel{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("reject delete", func(t *testing.T) {
		h := &validationHooks{rejectDelete: true}
		err := h.BeforeDelete(ctx, "1")
		if err == nil {
			t.Error("expected error from BeforeDelete")
		}
	})
}

func TestDefaultExitHooks_NoOp(t *testing.T) {
	var h DefaultExitHooks
	ctx := context.Background()

	msg, err := h.OnMessage(ctx, []byte("test"))
	if err != nil || string(msg) != "test" {
		t.Errorf("OnMessage: %v, %s", err, msg)
	}
	h.OnSuccess(ctx)
	h.OnError(ctx, context.DeadlineExceeded)
}

func TestService_WithHooks(t *testing.T) {
	svc := &Service{
		config:   &ServiceConfig{Name: "test", Port: 19040},
		handlers: &EntryHandlers{},
	}

	svc.WithHooks("Product", &trackingHooks{})

	if svc.hooks["Product"] == nil {
		t.Error("hooks not stored")
	}

	provider := &tableCRUD[testModel]{}
	svc.handlers.CRUD = map[string]CRUDProvider{"Order": provider}
	svc.WithHooks("Order", &trackingHooks{})
}

func TestTableCRUD_SetHooks(t *testing.T) {
	var h trackingHooks
	provider := &tableCRUD[testModel]{hooks: &h}

	newHooks := &trackingHooks{}
	provider.SetHooks(newHooks)

	if provider.hooks != newHooks {
		t.Error("SetHooks should replace existing hooks")
	}

	provider.SetHooks("not hooks")
	if provider.hooks != newHooks {
		t.Error("SetHooks with wrong type should not change hooks")
	}
}

func TestWrapTransformHandler(t *testing.T) {
	called := false
	handler := func(c fiber.Ctx) error {
		called = true
		return c.JSON(fiber.Map{"ok": true})
	}

	hooks := &transformHooks{modifiedName: "wrapped"}

	wrapped := WrapTransformHandler(handler, hooks)

	app := fiber.New()
	app.Post("/test", wrapped)

	req := httptest.NewRequestWithContext(context.Background(), "POST", "/test", strings.NewReader(`{"name":"original"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	if !called {
		t.Error("handler should have been called")
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if !hooks.beforeCalled {
		t.Error("BeforeTransform should have been called")
	}
	if !hooks.afterCalled {
		t.Error("AfterTransform should have been called")
	}

	// Verify c.Locals("transformed") was set with modified data
	// Test with GET (no body parsing)
	app2 := fiber.New()
	app2.Get("/get", wrapped)
	req2 := httptest.NewRequestWithContext(context.Background(), "GET", "/get", nil)
	resp2, _ := app2.Test(req2)
	if resp2.StatusCode != 200 {
		t.Errorf("GET status = %d", resp2.StatusCode)
	}
}
