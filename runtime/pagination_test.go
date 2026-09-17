package runtime

import (
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
)

func restCtxWithQuery(t *testing.T, raw string) *RestCtx {
	t.Helper()
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	t.Cleanup(func() { app.ReleaseCtx(fctx) })
	fctx.Request().Header.SetMethod("GET")
	fctx.Request().URI().SetQueryString(raw)
	return newRestCtx(fctx, nil)
}

func TestParseListParams_Defaults(t *testing.T) {
	p, err := ParseListParams(restCtxWithQuery(t, ""), 0, 0, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Page != 1 || p.Size != 10 || p.Sort != "id" || p.Pagination != "offset" {
		t.Errorf("defaults = %+v", p)
	}
	if len(p.Filters) != 0 {
		t.Errorf("filters = %v, want empty", p.Filters)
	}
}

func TestParseListParams_Values(t *testing.T) {
	p, err := ParseListParams(
		restCtxWithQuery(t, "page=3&size=25&sort=name&cursor=abc123&status=active"),
		10, 100, []string{"id", "name"}, "keyset",
	)
	if err != nil {
		t.Fatal(err)
	}
	if p.Page != 3 || p.Size != 25 || p.Sort != "name" {
		t.Errorf("values = %+v", p)
	}
	if p.Cursor != "abc123" || p.Pagination != "keyset" {
		t.Errorf("cursor/mode = %+v", p)
	}
	if p.Filters["status"] != "active" || len(p.Filters) != 1 {
		t.Errorf("filters = %v", p.Filters)
	}
}

func TestParseListParams_ClampAndFallback(t *testing.T) {
	p, err := ParseListParams(restCtxWithQuery(t, "page=0&size=9999&sort=x"), 10, 50, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Page != 1 || p.Size != 50 {
		t.Errorf("clamped = %+v", p)
	}
	p, err = ParseListParams(restCtxWithQuery(t, "page=abc&size=zz"), 10, 100, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Page != 1 || p.Size != 10 {
		t.Errorf("fallback = %+v", p)
	}
}

func TestParseListParams_BadSort(t *testing.T) {
	_, err := ParseListParams(restCtxWithQuery(t, "sort=drop"), 10, 100, []string{"id", "name"}, "")
	if err == nil {
		t.Error("expected validation error for unknown sort, got nil")
	}
}
