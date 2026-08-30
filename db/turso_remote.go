//go:build !golibsql

package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql" // driver "libsql" (hrana)
	_ "turso.tech/database/tursogo-serverless"           // driver "turso-serverless" (SQL over HTTP)
)

// Remote Turso/libSQL drivers for the default build (libsql-client-go linked).
// In the `golibsql` build, LibsqlOpen is replaced by go-libsql (see
// turso_remote_golibsql.go) because both register the "libsql" driver name.

func queryEscape(s string) string { return url.QueryEscape(s) }

// TursoServerlessOpen opens a remote Turso/libSQL database over HTTP using the
// tursogo-serverless driver (pure Go, no native libs, no CGO). The token is
// passed via ?auth_token= in the DSN (accepted by Turso Cloud and Bunny).
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

// LibsqlOpen opens a remote libSQL database using the libsql-client-go driver
// (hrana wire protocol). The token is passed via ?authToken= in the DSN.
func LibsqlOpen(databaseURL string, authToken string) (*sql.DB, error) {
	dsn := databaseURL
	if authToken != "" {
		dsn = dsnWithParam(databaseURL, "authToken", authToken)
	}
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: libsql open: %w", err)
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// TursoSyncOpen opens a local database synced to a remote Turso Cloud
// database (tursogo NewTursoSyncDb). Reads/writes are local; use Push/Pull
// for cloud sync. Only works with Turso Cloud (not Bunny).
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
