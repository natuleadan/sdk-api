//go:build golibsql && cgo

package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tursodatabase/go-libsql"
)

// GoLibsqlOpen opens an embedded replica synced to a remote libSQL primary
// using go-libsql. Reads are local; writes go to the cloud primary and are
// reflected back. Requires CGO (go-libsql bundles a native libSQL library).
//
// This build (tag `golibsql`) replaces libsql-client-go in the binary because
// both register the driver name "libsql" and cannot coexist. Build with:
//
//	go build -tags golibsql ./...
func GoLibsqlOpen(ctx context.Context, primaryURL string, authToken string) (*sql.DB, error) {
	dir, err := os.MkdirTemp("", "libsql-replica-*")
	if err != nil {
		return nil, fmt.Errorf("db: go-libsql tempdir: %w", err)
	}
	dbPath := filepath.Join(dir, "replica.db")

	opts := []libsql.Option{libsql.WithAuthToken(authToken)}
	connector, err := libsql.NewEmbeddedReplicaConnector(dbPath, primaryURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("db: go-libsql connector: %w", err)
	}

	db := sql.OpenDB(connector)
	db.SetConnMaxLifetime(0)
	return db, nil
}
