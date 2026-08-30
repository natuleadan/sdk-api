//go:build golibsql && !cgo

package db

import (
	"context"
	"database/sql"
	"fmt"
)

// GoLibsqlOpen is unavailable when CGO is disabled (go-libsql bundles a native
// library). Use turso-serverless or libsql drivers instead for remote access.
func GoLibsqlOpen(ctx context.Context, primaryURL string, authToken string) (*sql.DB, error) {
	return nil, fmt.Errorf("db: go-libsql requires CGO; use driver turso-serverless or libsql instead")
}
