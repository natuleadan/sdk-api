//go:build integration

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

func mysqlPool(t *testing.T) *MySQLTable[MySQLProduct] {
	t.Helper()
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	table, err := NewMySQLTableFromURL[MySQLProduct](url, "mysql_test_products")
	if err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	t.Cleanup(func() { table.Close() })
	if err := table.AutoInit(context.Background()); err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	return table
}

func TestMySQLTable(t *testing.T) {
	table := mysqlPool(t)
	ctx := context.Background()
	p := MySQLProduct{Name: "mysql-item", Price: 9.99}
	if err := table.Create(ctx, &p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
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

func TestMySQLTableList(t *testing.T) {
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	ctx := context.Background()
	table, err := NewMySQLTableFromURL[MySQLProduct](url, "mysql_test_list")
	if err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	defer table.Close()
	if err := table.AutoInit(ctx); err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	for _, name := range []string{"A", "B", "C"} {
		p := MySQLProduct{Name: name, Price: 10}
		if err := table.Create(ctx, &p); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}
	all, err := table.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	defer table.DB().ExecContext(ctx, "DROP TABLE IF EXISTS mysql_test_list")
}

func TestMySQLTableNotFound(t *testing.T) {
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	ctx := context.Background()
	table, err := NewMySQLTableFromURL[MySQLProduct](url, "mysql_test_notfound")
	if err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	defer table.Close()
	if err := table.AutoInit(ctx); err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	_, err = table.Get(ctx, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := table.Delete(ctx, 99999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on delete, got %v", err)
	}
	defer table.DB().ExecContext(ctx, "DROP TABLE IF EXISTS mysql_test_notfound")
}

func TestMySQLTableCount(t *testing.T) {
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	ctx := context.Background()
	table, err := NewMySQLTableFromURL[MySQLProduct](url, "mysql_test_count")
	if err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	defer table.Close()
	if err := table.AutoInit(ctx); err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	n, _ := table.Count(ctx)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	table.Create(ctx, &MySQLProduct{Name: "count-a", Price: 1})
	n, _ = table.Count(ctx)
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
	defer table.DB().ExecContext(ctx, "DROP TABLE IF EXISTS mysql_test_count")
}

func TestMySQLTablePaginated(t *testing.T) {
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	ctx := context.Background()
	table, err := NewMySQLTableFromURL[MySQLProduct](url, "mysql_test_paginated")
	if err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	defer table.Close()
	if err := table.AutoInit(ctx); err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	for i := range 5 {
		table.Create(ctx, &MySQLProduct{Name: "p", Price: float64(i)})
	}
	page, err := table.ListPaginated(ctx, 2, 1)
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if len(page) != 2 {
		t.Errorf("expected 2, got %d", len(page))
	}
	defer table.DB().ExecContext(ctx, "DROP TABLE IF EXISTS mysql_test_paginated")
}

func TestMySQLTableQueryKeyset(t *testing.T) {
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	ctx := context.Background()
	table, err := NewMySQLTableFromURL[MySQLProduct](url, "mysql_test_keyset")
	if err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	defer table.Close()
	if err := table.AutoInit(ctx); err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	for i := range 5 {
		table.Create(ctx, &MySQLProduct{Name: "k", Price: float64(i)})
	}
	items, next, err := table.QueryKeyset(ctx, "", 2, "id", nil)
	if err != nil {
		t.Fatalf("QueryKeyset: %v", err)
	}
	if len(items) != 2 || next == "" {
		t.Errorf("expected 2 with cursor, got %d next=%q", len(items), next)
	}
	defer table.DB().ExecContext(ctx, "DROP TABLE IF EXISTS mysql_test_keyset")
}

func TestMySQLTableCountScoped(t *testing.T) {
	type ScopedProduct struct {
		ID       int64  `db:"id,primary,auto"`
		TenantID string `db:"tenant_id,required"`
		Name     string `db:"name,required"`
	}
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	ctx := context.Background()
	table, err := NewMySQLTableFromURL[ScopedProduct](url, "mysql_test_count_scoped")
	if err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	defer table.Close()
	if err := table.AutoInit(ctx); err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	_ = table.Create(ctx, &ScopedProduct{TenantID: "t1", Name: "a"})
	_ = table.Create(ctx, &ScopedProduct{TenantID: "t1", Name: "b"})
	_ = table.Create(ctx, &ScopedProduct{TenantID: "t2", Name: "c"})
	n, _ := table.CountScoped(ctx, "tenant_id", "t1")
	if n != 2 {
		t.Errorf("expected 2 for t1, got %d", n)
	}
	defer table.DB().ExecContext(ctx, "DROP TABLE IF EXISTS mysql_test_count_scoped")
}

func TestMySQLTableValidColumn(t *testing.T) {
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	ctx := context.Background()
	table, err := NewMySQLTableFromURL[MySQLProduct](url, "mysql_test_validcol")
	if err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	defer table.Close()
	if err := table.AutoInit(ctx); err != nil {
		t.Skipf("MySQL not available (%v), skipping", err)
	}
	if _, err := table.GetScoped(ctx, int64(1), "nonexistent", "x"); err == nil {
		t.Error("expected error for invalid column")
	}
	defer table.DB().ExecContext(ctx, "DROP TABLE IF EXISTS mysql_test_validcol")
}
