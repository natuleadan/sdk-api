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

// OpenForDriver opens a *sql.DB using the driver's canonical constructor,
// including the out-of-band auth token that remote Turso/libSQL drivers need.
// It is the entry point for tools that must reach any supported family (the
// migration runner, for example) without duplicating the DSN wiring.
func OpenForDriver(driver, dsn, authToken string) (*sql.DB, error) {
	switch driver {
	case "turso-serverless":
		return TursoServerlessOpen(dsn, authToken)
	case "libsql":
		return LibsqlOpen(dsn, authToken)
	default:
		if authToken != "" {
			return nil, fmt.Errorf("db: driver %q does not take an auth token", driver)
		}
		return OpenStdlib(driver, dsn)
	}
}
