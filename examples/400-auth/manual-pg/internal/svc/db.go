package svc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

// currentDriver is the YAML database driver of the "primary" database. The
// example is DB-agnostic: handlers query through DB(c) and the SQL is
// translated per dialect.
var currentDriver = "postgres"

// SetDriver records the runtime database driver (from DB_DRIVER).
func SetDriver(d string) {
	if d != "" {
		currentDriver = d
	}
}

// Driver returns the current database driver.
func Driver() string { return currentDriver }

// Row, Rows and CommandTag mirror the small subset of pgx used by the handlers,
// so the same code compiles over pgxpool (Postgres) and database/sql (Turso,
// MySQL).
type Row interface{ Scan(dest ...any) error }

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

type CommandTag interface{ RowsAffected() int64 }

// Querier runs queries against the primary database, rewriting PostgreSQL
// syntax ($n placeholders, now(), gen_random_uuid()) to the active dialect.
type Querier struct {
	dialect db.SQLDialect
	pg      *pgxpool.Pool
	sql     *sql.DB
}

// DB returns the driver-agnostic query handle for the "primary" database.
func DB(c *runtime.RestCtx) *Querier {
	d := db.DialectForDriver(currentDriver)
	q := &Querier{dialect: d}
	if d == db.DialectPostgres {
		q.pg = c.PoolPG("primary")
	} else {
		q.sql = c.PoolSQL("primary")
	}
	return q
}

// DBOf is like DB but for code without a RestCtx (seed, auth validators).
func DBOf(s *runtime.Service) *Querier {
	d := db.DialectForDriver(currentDriver)
	q := &Querier{dialect: d}
	if d == db.DialectPostgres {
		q.pg = s.PoolPGTyped("primary")
	} else {
		q.sql = s.PoolSQLTyped("primary")
	}
	return q
}

// Dialect exposes the active SQL dialect (for dynamic clauses).
func (q *Querier) Dialect() db.SQLDialect { return q.dialect }

func (q *Querier) Exec(ctx context.Context, query string, args ...any) (CommandTag, error) {
	query, args = db.Rewrite(q.dialect, query, args...)
	switch {
	case q.pg != nil:
		return q.pg.Exec(ctx, query, args...)
	case q.sql != nil:
		res, err := q.sql.ExecContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", n)), nil
	default:
		return nil, errors.New("no database pool")
	}
}

func (q *Querier) QueryRow(ctx context.Context, query string, args ...any) Row {
	query, args = db.Rewrite(q.dialect, query, args...)
	switch {
	case q.pg != nil:
		return q.pg.QueryRow(ctx, query, args...)
	case q.sql != nil:
		return q.sql.QueryRowContext(ctx, query, args...)
	default:
		return errRow{errors.New("no database pool")}
	}
}

func (q *Querier) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	query, args = db.Rewrite(q.dialect, query, args...)
	switch {
	case q.pg != nil:
		rows, err := q.pg.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		return pgRows{rows}, nil
	case q.sql != nil:
		return q.sql.QueryContext(ctx, query, args...)
	default:
		return nil, errors.New("no database pool")
	}
}

// pgRows adapts pgx.Rows (Close returns nothing) to the shared Rows interface.
type pgRows struct{ rows pgx.Rows }

func (r pgRows) Next() bool             { return r.rows.Next() }
func (r pgRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r pgRows) Close() error           { r.rows.Close(); return nil }
func (r pgRows) Err() error             { return r.rows.Err() }

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }
