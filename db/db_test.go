// Package db provides database table abstractions and ORM-like CRUD operations.
package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://dev:devpass@localhost:5432/postgres?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres not reachable: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type testProduct struct {
	ID        int64     `db:"id,primary,auto"`
	Name      string    `db:"name,required"`
	Price     float64   `db:"price"`
	Stock     int       `db:"stock,default=0"`
	CreatedAt time.Time `db:"created_at,default=NOW()"`
}

func TestTableLifecycle(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	table, err := NewTable[testProduct](pool, "test_products")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	pool.Exec(ctx, `DROP TABLE IF EXISTS test_products`)

	if err := table.AutoInit(ctx); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	created := testProduct{Name: "Laptop", Price: 999.99, Stock: 10}
	if err := table.Create(ctx, &created); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected non-zero ID after Create")
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("expected non-zero CreatedAt, got zero time")
	}

	got, err := table.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Laptop" {
		t.Errorf("expected Name=Laptop, got %q", got.Name)
	}
	if got.Price != 999.99 {
		t.Errorf("expected Price=999.99, got %f", got.Price)
	}

	updated, err := table.Update(ctx, created.ID, map[string]any{
		"price": 899.99,
		"stock": 5,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Price != 899.99 {
		t.Errorf("expected Price=899.99 after update, got %f", updated.Price)
	}
	if updated.Stock != 5 {
		t.Errorf("expected Stock=5 after update, got %d", updated.Stock)
	}

	if err := table.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = table.Get(ctx, created.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestTableList(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	table, err := NewTable[testProduct](pool, "test_products_list")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	pool.Exec(ctx, `DROP TABLE IF EXISTS test_products_list`)

	if err := table.AutoInit(ctx); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	for _, p := range []testProduct{
		{Name: "A", Price: 10},
		{Name: "B", Price: 20},
		{Name: "C", Price: 30},
	} {
		if err := table.Create(ctx, &p); err != nil {
			t.Fatalf("Create %s: %v", p.Name, err)
		}
	}

	all, err := table.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 products, got %d", len(all))
	}
}

func TestTableNotFound(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	table, err := NewTable[testProduct](pool, "test_not_found")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	pool.Exec(ctx, `DROP TABLE IF EXISTS test_not_found`)
	if err := table.AutoInit(ctx); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}

	_, err = table.Get(ctx, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	err = table.Delete(ctx, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on delete, got %v", err)
	}
}

func TestTableUpdateNoFields(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	table, err := NewTable[testProduct](pool, "test_update_no_fields")
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	_, err = table.Update(ctx, 1, map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty update")
	}
}

func TestTableCount(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_count")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_count")
	table.AutoInit(ctx)
	n, err := table.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	table.Create(ctx, &testProduct{Name: "A", Price: 10})
	n, _ = table.Count(ctx)
	if n != 1 {
		t.Errorf("expected 1 after insert, got %d", n)
	}
}

func TestTableExists(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_exists")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_exists")
	table.AutoInit(ctx)
	p := testProduct{Name: "exists-test", Price: 5}
	table.Create(ctx, &p)
	ok, err := table.Exists(ctx, "name", "exists-test")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Error("expected true for existing record")
	}
	ok, _ = table.Exists(ctx, "name", "nope")
	if ok {
		t.Error("expected false for non-existing record")
	}
}

func TestTableIncrement(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_inc")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_inc")
	table.AutoInit(ctx)
	p := testProduct{Name: "inc-test", Price: 10, Stock: 5}
	table.Create(ctx, &p)
	err := table.Increment(ctx, p.ID, "stock", 3)
	if err != nil {
		t.Fatalf("Increment: %v", err)
	}
	got, _ := table.Get(ctx, p.ID)
	if got.Stock != 8 {
		t.Errorf("expected stock=8, got %d", got.Stock)
	}
}

func TestTableTransaction(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_tx")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_tx")
	table.AutoInit(ctx)
	err := table.Transaction(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO test_tx (name, price) VALUES ($1, $2)", "tx-test", 100.0)
		return e
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	n, _ := table.Count(ctx)
	if n != 1 {
		t.Errorf("expected 1 record, got %d", n)
	}
}

func TestTableBatchInsert(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_batch")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_batch")
	table.AutoInit(ctx)
	items := []testProduct{
		{Name: "batch-A", Price: 10, Stock: 1},
		{Name: "batch-B", Price: 20, Stock: 2},
		{Name: "batch-C", Price: 30, Stock: 3},
	}
	err := table.BatchInsert(ctx, items)
	if err != nil {
		t.Fatalf("BatchInsert: %v", err)
	}
	n, _ := table.Count(ctx)
	if n != 3 {
		t.Errorf("expected 3 records, got %d", n)
	}
}

func TestTablePaginated(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_paginated")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_paginated")
	table.AutoInit(ctx)
	for i := range 10 {
		table.Create(ctx, &testProduct{Name: fmt.Sprintf("p%d", i), Price: float64(i)})
	}
	items, total, err := table.QueryPaginated(ctx, 1, 3, "id")
	if err != nil {
		t.Fatalf("QueryPaginated: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total=10, got %d", total)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestTableQueryKeyset(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_keyset")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_keyset")
	table.AutoInit(ctx)
	for i := range 10 {
		table.Create(ctx, &testProduct{Name: fmt.Sprintf("p%d", i), Price: float64(i)})
	}

	// First page (no cursor)
	items, next, err := table.QueryKeyset(ctx, "", 4, "id", nil)
	if err != nil {
		t.Fatalf("QueryKeyset page1: %v", err)
	}
	if len(items) != 4 {
		t.Errorf("page1: expected 4 items, got %d", len(items))
	}
	if next == "" {
		t.Error("page1: expected non-empty nextCursor")
	}

	// Second page with cursor
	items, next, err = table.QueryKeyset(ctx, next, 4, "id", nil)
	if err != nil {
		t.Fatalf("QueryKeyset page2: %v", err)
	}
	if len(items) != 4 {
		t.Errorf("page2: expected 4 items, got %d", len(items))
	}
	if next == "" {
		t.Error("page2: expected non-empty nextCursor")
	}

	// Third page (last)
	items, next, err = table.QueryKeyset(ctx, next, 4, "id", nil)
	if err != nil {
		t.Fatalf("QueryKeyset page3: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("page3: expected 2 items, got %d", len(items))
	}
	if next != "" {
		t.Errorf("page3: expected empty nextCursor, got %q", next)
	}

	// Test with filter
	items, _, err = table.QueryKeyset(ctx, "", 5, "id", map[string]any{"price": 3.0})
	if err != nil {
		t.Fatalf("QueryKeyset filter: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("filter: expected 1 item, got %d", len(items))
	}
}

type testUpsertModel struct {
	ID    int64   `db:"id,primary,auto"`
	Name  string  `db:"name,unique"`
	Price float64 `db:"price"`
}

func TestTableUpsert(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testUpsertModel](pool, "test_upsert")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_upsert")
	table.AutoInit(ctx)
	p := testUpsertModel{Name: "upsert-test", Price: 50}
	err := table.Upsert(ctx, &p, "name")
	if err != nil {
		t.Fatalf("Upsert insert: %v", err)
	}
	p.Price = 99
	err = table.Upsert(ctx, &p, "name")
	if err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	got, _ := table.FindBy(ctx, "name", "upsert-test")
	if got.Price != 99 {
		t.Errorf("expected price=99 after upsert, got %f", got.Price)
	}
}

func TestTableQueryWhere(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_where")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_where")
	table.AutoInit(ctx)
	table.Create(ctx, &testProduct{Name: "apple", Price: 1})
	table.Create(ctx, &testProduct{Name: "banana", Price: 2})
	table.Create(ctx, &testProduct{Name: "cherry", Price: 3})
	items, err := table.QueryWhere(ctx, map[string]any{"price": 2}, "id", 0, 0)
	if err != nil {
		t.Fatalf("QueryWhere: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if len(items) > 0 && items[0].Name != "banana" {
		t.Errorf("expected banana, got %s", items[0].Name)
	}
}

func TestTableExecRaw(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_exec_raw")
	pool.Exec(ctx, "DROP TABLE IF EXISTS test_exec_raw")
	table.AutoInit(ctx)
	table.Create(ctx, &testProduct{Name: "raw-test", Price: 10})
	n, err := table.ExecRaw(ctx, "UPDATE test_exec_raw SET price = $1 WHERE name = $2", 99.0, "raw-test")
	if err != nil {
		t.Fatalf("ExecRaw: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row affected, got %d", n)
	}
}

// --- Column Validation Tests ---

func TestValidColumnValid(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_column_valid")
	defer pool.Exec(ctx, "DROP TABLE IF EXISTS test_column_valid")

	_, err := table.validColumn("name")
	if err != nil {
		t.Errorf("expected 'name' to be valid, got %v", err)
	}

	_, err = table.validColumn("price")
	if err != nil {
		t.Errorf("expected 'price' to be valid, got %v", err)
	}
}

func TestValidColumnInvalid(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	table, _ := NewTable[testProduct](pool, "test_column_invalid")
	defer pool.Exec(ctx, "DROP TABLE IF EXISTS test_column_invalid")

	_, err := table.validColumn("nonexistent")
	if err == nil {
		t.Error("expected error for invalid column")
	}

	_, err = table.validColumn("'; DROP TABLE users; --")
	if err == nil {
		t.Error("expected error for SQL injection attempt")
	}
}
