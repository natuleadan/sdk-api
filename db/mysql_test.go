package db

import (
	"context"
	"os"
	"testing"
)

type MySQLProduct struct {
	ID    int64   `db:"id,primary,auto"`
	Name  string  `db:"name,required"`
	Price float64 `db:"price"`
}

func TestMySQLTable(t *testing.T) {
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}

	ctx := context.Background()
	table, err := NewMySQLTableFromURL[MySQLProduct](url, "mysql_test_products")
	if err != nil {
		t.Fatalf("new table: %v", err)
	}
	defer table.Close()

	if err := table.AutoInit(ctx); err != nil {
		t.Fatalf("autoinit: %v", err)
	}

	p := MySQLProduct{Name: "mysql-item", Price: 9.99}
	if err := table.Create(ctx, &p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	t.Logf("created id=%d", p.ID)

	got, err := table.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "mysql-item" {
		t.Errorf("expected mysql-item, got %q", got.Name)
	}

	updated, err := table.Update(ctx, p.ID, map[string]any{"price": 19.99})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Price != 19.99 {
		t.Errorf("expected price 19.99, got %f", updated.Price)
	}

	list, err := table.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 item, got %d", len(list))
	}

	if err := table.Delete(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := table.Get(ctx, p.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
