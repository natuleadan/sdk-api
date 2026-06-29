package db

import (
	"context"
	"path/filepath"
	"testing"
)

type TursoProduct struct {
	ID    int64   `db:"id,primary,auto"`
	Name  string  `db:"name,required"`
	Price float64 `db:"price"`
	Stock int     `db:"stock,default=0"`
}

func TestTursoTableLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "lifecycle.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_products")
	if err != nil {
		t.Fatalf("NewTursoTable: %v", err)
	}
	defer table.Close()

	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	p := TursoProduct{Name: "Widget", Price: 19.99, Stock: 100}
	if err := table.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected auto-incremented ID")
	}
	t.Logf("created id=%d", p.ID)

	got, err := table.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Widget" {
		t.Errorf("expected Widget, got %q", got.Name)
	}
	if got.Price != 19.99 {
		t.Errorf("expected 19.99, got %f", got.Price)
	}
	if got.Stock != 100 {
		t.Errorf("expected Stock 100, got %d", got.Stock)
	}
}

func TestTursoTableCreateAutoID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "autoid.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_autoid")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	p1 := TursoProduct{Name: "A", Price: 1.0}
	p2 := TursoProduct{Name: "B", Price: 2.0}

	if err := table.Create(ctx, &p1); err != nil {
		t.Fatal(err)
	}
	if err := table.Create(ctx, &p2); err != nil {
		t.Fatal(err)
	}

	if p1.ID == p2.ID {
		t.Fatal("expected different IDs")
	}
	if p2.ID != p1.ID+1 {
		t.Errorf("expected sequential IDs, got %d then %d", p1.ID, p2.ID)
	}
}

func TestTursoTableList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "list.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_list")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	names := []string{"Alpha", "Beta", "Gamma"}
	for _, name := range names {
		p := TursoProduct{Name: name, Price: 10.0}
		if err := table.Create(ctx, &p); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	list, err := table.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list))
	}
	if list[0].Name != "Alpha" {
		t.Errorf("expected first Alpha, got %q", list[0].Name)
	}
}

func TestTursoTableUpdate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "update.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_update")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	p := TursoProduct{Name: "OldName", Price: 5.0, Stock: 10}
	if err := table.Create(ctx, &p); err != nil {
		t.Fatal(err)
	}

	updated, err := table.Update(ctx, p.ID, map[string]any{"price": 15.0})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Price != 15.0 {
		t.Errorf("expected price 15.0, got %f", updated.Price)
	}
	if updated.Name != "OldName" {
		t.Errorf("expected name OldName, got %q", updated.Name)
	}
}

func TestTursoTableDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "delete.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_delete")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	p := TursoProduct{Name: "DeleteMe", Price: 1.0}
	if err := table.Create(ctx, &p); err != nil {
		t.Fatal(err)
	}

	if err := table.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = table.Get(ctx, p.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestTursoTableGetNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "notfound.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_notfound")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = table.Get(ctx, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTursoTableDeleteNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "delnotfound.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_delnotfound")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	err = table.Delete(ctx, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTursoTableDefaultValues(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "defaults.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_defaults")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	p := TursoProduct{Name: "Defaults", Price: 1.0}
	if err := table.Create(ctx, &p); err != nil {
		t.Fatal(err)
	}

	got, err := table.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stock != 0 {
		t.Errorf("expected default stock 0, got %d", got.Stock)
	}
}

func TestTursoTableMultipleFieldsUpdate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "multiupdate.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_multiupdate")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	p := TursoProduct{Name: "Original", Price: 10.0, Stock: 5}
	if err := table.Create(ctx, &p); err != nil {
		t.Fatal(err)
	}

	updated, err := table.Update(ctx, p.ID, map[string]any{
		"name":  "Updated",
		"price": 25.0,
		"stock": 50,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Updated" || updated.Price != 25.0 || updated.Stock != 50 {
		t.Errorf("got %+v", updated)
	}
}

func TestTursoTableUpdateNoFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nofields.db")
	table, err := NewTursoTable[TursoProduct](dbPath, "test_nofields")
	if err != nil {
		t.Fatal(err)
	}
	defer table.Close()
	ctx := context.Background()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = table.Update(ctx, 1, map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty update")
	}
}

type TursoNoPK struct {
	Name string `db:"name"`
}

func TestTursoTableNoPrimaryKey(t *testing.T) {
	_, err := NewTursoTable[TursoNoPK]("test.db", "test_nopk")
	if err == nil {
		t.Fatal("expected error for struct without primary key")
	}
}
