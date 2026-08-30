//go:build !golibsql

package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Turso drivers integration tests.
// Local variants run on a TempDir (no external services).
// Remote variants (turso-serverless, libsql) run only when TURSO_DATABASE_URL
// and TURSO_AUTH_TOKEN are set (Turso Cloud testing DB) or
// BUNNY_DATABASE_URL/BUNNY_DATABASE_AUTH_TOKEN (Bunny).

func remoteTursoEnv(t *testing.T) (string, string) {
	t.Helper()
	url := os.Getenv("TURSO_DATABASE_URL")
	tok := os.Getenv("TURSO_AUTH_TOKEN")
	if url == "" {
		url = os.Getenv("BUNNY_DATABASE_URL")
		tok = os.Getenv("BUNNY_DATABASE_AUTH_TOKEN")
	}
	if url == "" || tok == "" {
		t.Skip("remote Turso/libSQL env not set (TURSO_DATABASE_URL/TURSO_AUTH_TOKEN)")
	}
	return url, tok
}

func driverLifecycle(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS drv_test (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO drv_test (name) VALUES (?)", "hello"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var name string
	if err := db.QueryRowContext(ctx,
		"SELECT name FROM drv_test ORDER BY id DESC LIMIT 1").Scan(&name); err != nil {
		t.Fatalf("query: %v", err)
	}
	if name != "hello" {
		t.Fatalf("name = %q, want hello", name)
	}
	// Transactions
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO drv_test (name) VALUES (?)", "tx"); err != nil {
		_ = tx.Rollback()
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("tx commit: %v", err)
	}
}

// --- Variant 1: tursogo local (embedded, plain path) ---

func TestTursoLocalDriver(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "local.db")
	db, err := TursoOpen(dbPath)
	if err != nil {
		t.Fatalf("TursoOpen: %v", err)
	}
	defer db.Close()
	driverLifecycle(t, db)
}

// --- Variant 1b: tursogo sync vs Turso Cloud ---

func TestTursoSyncDriver(t *testing.T) {
	url, tok := remoteTursoEnv(t)
	if len(url) > 0 && len(tok) > 0 {
		// Only Turso Cloud supports sync; Bunny returns 404.
		if len(os.Getenv("TURSO_DATABASE_URL")) == 0 {
			t.Skip("sync only supported on Turso Cloud (TURSO_DATABASE_URL)")
		}
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sync.db")
	db, err := TursoSyncOpen(ctx, path, url, tok)
	if err != nil {
		t.Skipf("sync unavailable: %v", err)
	}
	defer db.Close()
	driverLifecycle(t, db)
}

// --- Variant 2: tursogo-serverless (remote, pure Go) ---

func TestTursoServerlessDriver(t *testing.T) {
	url, tok := remoteTursoEnv(t)
	db, err := TursoServerlessOpen(url, tok)
	if err != nil {
		t.Fatalf("TursoServerlessOpen: %v", err)
	}
	defer db.Close()
	driverLifecycle(t, db)
}

// --- Variant 3: libsql-client-go (remote hrana) ---

func TestLibsqlDriver(t *testing.T) {
	url, tok := remoteTursoEnv(t)
	db, err := LibsqlOpen(url, tok)
	if err != nil {
		t.Fatalf("LibsqlOpen: %v", err)
	}
	defer db.Close()
	driverLifecycle(t, db)
}

// --- DSN helper ---

func TestDSNWithParam(t *testing.T) {
	if got := dsnWithParam("libsql://db.turso.io", "auth_token", "tok/abc"); got != "libsql://db.turso.io?auth_token=tok%2Fabc" {
		t.Fatalf("dsnWithParam = %q", got)
	}
	if got := dsnWithParam("libsql://db.turso.io?x=1", "authToken", "tok"); got != "libsql://db.turso.io?x=1&authToken=tok" {
		t.Fatalf("dsnWithParam append = %q", got)
	}
}
