package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/valyala/fasthttp"
)

type filterLink struct {
	ID        int64  `db:"id,primary,auto" json:"id"`
	ShortCode string `db:"short_code" json:"shortCode"`
	TargetURL string `db:"target_url" json:"targetUrl"`
}

func tursoFilterProvider(t *testing.T) CRUDProvider {
	t.Helper()
	tbl, err := db.NewTursoTable[filterLink](filepath.Join(t.TempDir(), "filter.db"), "filter_link")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tbl.Close() })
	ctx := context.Background()
	if err := tbl.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, l := range []filterLink{
		{ShortCode: "aaa", TargetURL: "https://a.example.com"},
		{ShortCode: "bbb", TargetURL: "https://b.example.com"},
	} {
		row := l
		if err := tbl.Create(ctx, &row); err != nil {
			t.Fatal(err)
		}
	}
	return NewTursoCRUDProvider(tbl, nil)
}

func TestCRUDList_JSONFilterKeys(t *testing.T) {
	provider := tursoFilterProvider(t)
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("GET")
	rc := newRestCtx(fctx, nil)

	err := provider.List(rc, ListParams{
		Page: 1, Size: 10, Sort: "id", Pagination: "offset",
		Filters: map[string]string{"shortCode": "bbb"},
	})
	if err != nil {
		t.Fatalf("List with JSON filter key: %v", err)
	}
	body := rc.ResponseBody()
	if !strings.Contains(body, "b.example.com") {
		t.Errorf("expected filtered row, got %s", body)
	}
	if strings.Contains(body, "a.example.com") {
		t.Errorf("unfiltered row leaked: %s", body)
	}
}
