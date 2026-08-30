//go:build golibsql

package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "turso.tech/database/tursogo-serverless" // driver "turso-serverless" (SQL over HTTP)
)

// Remote Turso/libSQL drivers for the `golibsql` build. libsql-client-go is
// NOT linked here because go-libsql registers the same driver name "libsql"
// (see turso_golibsql.go). TursoServerlessOpen / TursoSyncOpen are available
// (their drivers don't collide).

func queryEscape(s string) string { return url.QueryEscape(s) }

// TursoServerlessOpen opens a remote Turso/libSQL database over HTTP using the
// tursogo-serverless driver (pure Go). Available in both builds.
func TursoServerlessOpen(databaseURL string, authToken string) (*sql.DB, error) {
	dsn := databaseURL
	if authToken != "" {
		dsn = dsnWithParam(databaseURL, "auth_token", authToken)
	}
	db, err := sql.Open("turso-serverless", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: turso-serverless open: %w", err)
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// LibsqlOpen is unavailable in the `golibsql` build (go-libsql owns the
// "libsql" driver). Use GoLibsqlOpen or TursoServerlessOpen instead.
func LibsqlOpen(databaseURL string, authToken string) (*sql.DB, error) {
	return nil, fmt.Errorf("db: libsql-client-go not linked in golibsql build; use GoLibsqlOpen or TursoServerlessOpen")
}

// TursoSyncOpen opens a local database synced to a remote Turso Cloud
// database (tursogo NewTursoSyncDb).
func TursoSyncOpen(ctx context.Context, path string, remoteURL string, authToken string) (*sql.DB, error) {
	turso, err := newTursoSyncDB(ctx, path, remoteURL, authToken)
	if err != nil {
		return nil, fmt.Errorf("db: turso sync: %w", err)
	}
	db, err := turso.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("db: turso sync connect: %w", err)
	}
	return db, nil
}

// DSN builders shared by tests.
func dsnWithParam(dsn, param, value string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + param + "=" + queryEscape(value)
}
