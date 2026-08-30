//go:build !golibsql

package db

import (
	"context"
	"database/sql"
	"fmt"
)

// GoLibsqlOpen is unavailable in the default build (libsql-client-go is
// linked instead; both register the "libsql" driver). Build with -tags
// golibsql (and CGO) to use go-libsql embedded replicas.
func GoLibsqlOpen(ctx context.Context, primaryURL string, authToken string) (*sql.DB, error) {
	return nil, fmt.Errorf("db: go-libsql not included; build with -tags golibsql (requires CGO)")
}
