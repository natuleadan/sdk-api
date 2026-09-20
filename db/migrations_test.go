package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// newMigratorTestDB opens a local Turso database in a temp dir. The migrator is
// driver-agnostic (*sql.DB), so the same runner is exercised here on the default
// embedded engine; PostgreSQL and MySQL share the code path.
func newMigratorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migrate.db")
	database, err := TursoOpen(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func writeMigration(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write migration %s: %v", name, err)
	}
}

func TestMigratorUpAppliesInOrder(t *testing.T) {
	ctx := context.Background()
	database := newMigratorTestDB(t)
	dir := t.TempDir()
	writeMigration(t, dir, "0002_add_email.sql", "ALTER TABLE users ADD COLUMN email TEXT;")
	writeMigration(t, dir, "0001_create_users.sql", "CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT);")

	m, err := NewMigrator(database, dir)
	if err != nil {
		t.Fatalf("new migrator: %v", err)
	}
	done, err := m.Up(ctx)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(done) != 2 || done[0] != 1 || done[1] != 2 {
		t.Fatalf("applied versions = %v, want [1 2]", done)
	}
	// The column exists.
	if _, err := database.ExecContext(ctx, "INSERT INTO users (id, name, email) VALUES ('u1','n','e')"); err != nil {
		t.Fatalf("insert with email: %v", err)
	}
}

func TestMigratorUpIsIdempotent(t *testing.T) {
	ctx := context.Background()
	database := newMigratorTestDB(t)
	dir := t.TempDir()
	writeMigration(t, dir, "0001_create.sql", "CREATE TABLE t1 (id TEXT PRIMARY KEY);")

	m, err := NewMigrator(database, dir)
	if err != nil {
		t.Fatalf("new migrator: %v", err)
	}
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("first up: %v", err)
	}
	done, err := m.Up(ctx)
	if err != nil {
		t.Fatalf("second up: %v", err)
	}
	if len(done) != 0 {
		t.Fatalf("second up applied %v, want none", done)
	}
}

func TestMigratorStatusReportsAppliedAndPending(t *testing.T) {
	ctx := context.Background()
	database := newMigratorTestDB(t)
	dir := t.TempDir()
	writeMigration(t, dir, "0001_create.sql", "CREATE TABLE t1 (id TEXT PRIMARY KEY);")

	m, err := NewMigrator(database, dir)
	if err != nil {
		t.Fatalf("new migrator: %v", err)
	}
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}
	// Add a new pending file after the first run.
	writeMigration(t, dir, "0002_more.sql", "CREATE TABLE t2 (id TEXT PRIMARY KEY);")
	m, err = NewMigrator(database, dir)
	if err != nil {
		t.Fatalf("reload migrator: %v", err)
	}
	status, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status) != 2 {
		t.Fatalf("status len = %d, want 2", len(status))
	}
	if !status[0].Applied || status[1].Applied {
		t.Fatalf("status applied flags = %v, want [true false]", []bool{status[0].Applied, status[1].Applied})
	}
}

func TestMigratorDetectsChangedAppliedFile(t *testing.T) {
	ctx := context.Background()
	database := newMigratorTestDB(t)
	dir := t.TempDir()
	writeMigration(t, dir, "0001_create.sql", "CREATE TABLE t1 (id TEXT PRIMARY KEY);")

	m, _ := NewMigrator(database, dir)
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}
	// Rewrite the applied file: the checksum must no longer match.
	writeMigration(t, dir, "0001_create.sql", "CREATE TABLE t1 (id TEXT PRIMARY KEY, extra TEXT);")
	m, _ = NewMigrator(database, dir)
	if _, err := m.Up(ctx); err == nil {
		t.Fatal("up after modifying an applied migration must fail")
	}
	status, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status[0].Modified {
		t.Fatal("status must flag the modified applied migration")
	}
}

func TestMigratorDownUsesCompanionFile(t *testing.T) {
	ctx := context.Background()
	database := newMigratorTestDB(t)
	dir := t.TempDir()
	writeMigration(t, dir, "0001_create.sql", "CREATE TABLE t1 (id TEXT PRIMARY KEY);")
	writeMigration(t, dir, "0001_create.down.sql", "DROP TABLE t1;")

	m, _ := NewMigrator(database, dir)
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}
	version, err := m.Down(ctx)
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if version != 1 {
		t.Fatalf("rolled back version = %d, want 1", version)
	}
	// Table is gone: selecting from it fails.
	if _, err := database.ExecContext(ctx, "SELECT * FROM t1"); err == nil {
		t.Fatal("t1 still exists after down")
	}
}

func TestMigratorRejectsDuplicateVersions(t *testing.T) {
	database := newMigratorTestDB(t)
	dir := t.TempDir()
	writeMigration(t, dir, "0001_a.sql", "SELECT 1;")
	writeMigration(t, dir, "0001_b.sql", "SELECT 1;")
	if _, err := NewMigrator(database, dir); err == nil {
		t.Fatal("duplicate versions must be rejected")
	}
}

func TestSplitStatementsSkipsComments(t *testing.T) {
	sqlText := "-- a comment\nCREATE TABLE a (id TEXT);\n\n-- another\nCREATE TABLE b (id TEXT);\n"
	stmts := splitStatements(sqlText)
	if len(stmts) != 2 {
		t.Fatalf("statements = %d, want 2 (%v)", len(stmts), stmts)
	}
}
