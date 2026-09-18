package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// OpenStdlib opens a *sql.DB for the given YAML driver and DSN, so code that
// must be driver-agnostic (e.g. the fosite store) can use database/sql for
// every family. Postgres uses the pgx stdlib driver; MySQL and Turso/libSQL use
// the drivers registered elsewhere in this package.
func OpenStdlib(driver, dsn string) (*sql.DB, error) {
	name := driver
	switch driver {
	case "postgres":
		name = "pgx"
	case "mysql", "mariadb":
		name = "mysql"
	case "turso":
		name = "turso"
	case "turso-serverless":
		name = "turso-serverless"
	case "libsql", "go-libsql":
		name = "libsql"
	}
	database, err := sql.Open(name, dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", driver, err)
	}
	return database, nil
}
