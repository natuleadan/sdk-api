package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDialectForDriver(t *testing.T) {
	assert.Equal(t, DialectPostgres, DialectForDriver("postgres"))
	assert.Equal(t, DialectMySQL, DialectForDriver("mysql"))
	assert.Equal(t, DialectMySQL, DialectForDriver("mariadb"))
	assert.Equal(t, DialectSQLite, DialectForDriver("turso"))
	assert.Equal(t, DialectSQLite, DialectForDriver("turso-serverless"))
	assert.Equal(t, DialectSQLite, DialectForDriver("libsql"))
	assert.Equal(t, DialectSQLite, DialectForDriver("go-libsql"))
}

func TestRewrite(t *testing.T) {
	// Postgres is a no-op.
	q, args := Rewrite(DialectPostgres, "SELECT 1 WHERE a = $1 AND b = $2", 1, 2)
	assert.Equal(t, "SELECT 1 WHERE a = $1 AND b = $2", q)
	assert.Equal(t, []any{1, 2}, args)

	// Positional: $n becomes ? and arguments follow occurrence order.
	q, args = Rewrite(DialectSQLite, "INSERT INTO t (a, b) VALUES ($1, $2)", "x", "y")
	assert.Equal(t, "INSERT INTO t (a, b) VALUES (?, ?)", q)
	assert.Equal(t, []any{"x", "y"}, args)

	// A repeated $1 duplicates the argument for positional dialects.
	q, args = Rewrite(DialectMySQL, "UPDATE t SET a = $1 WHERE a = $1 OR b = $2", "x", "y")
	assert.Equal(t, "UPDATE t SET a = ? WHERE a = ? OR b = ?", q)
	assert.Equal(t, []any{"x", "x", "y"}, args)

	// Placeholders inside string literals are untouched.
	q, _ = Rewrite(DialectSQLite, "SELECT '$1' WHERE a = $1", 1)
	assert.Equal(t, "SELECT '$1' WHERE a = ?", q)
}

func TestRewriteExpressions(t *testing.T) {
	q, args := Rewrite(DialectSQLite, "UPDATE t SET a = now() WHERE b = $1", 1)
	assert.Equal(t, "UPDATE t SET a = CURRENT_TIMESTAMP WHERE b = ?", q)
	assert.Equal(t, []any{1}, args)

	q, _ = Rewrite(DialectMySQL, "INSERT INTO t (id) VALUES (gen_random_uuid())")
	assert.Equal(t, "INSERT INTO t (id) VALUES ((UUID()))", q)

	q, _ = Rewrite(DialectSQLite, "SELECT gen_random_uuid()")
	assert.Equal(t, "SELECT (lower(hex(randomblob(16))))", q)

	// Inside quotes, and non-call identifiers, are untouched.
	q, _ = Rewrite(DialectSQLite, "SELECT 'now()' AS x, nowhere FROM t")
	assert.Equal(t, "SELECT 'now()' AS x, nowhere FROM t", q)

	// Postgres keeps the pivot syntax.
	q, _ = Rewrite(DialectPostgres, "SELECT now(), gen_random_uuid()")
	assert.Equal(t, "SELECT now(), gen_random_uuid()", q)
}

func TestQueryExpressions(t *testing.T) {
	assert.Equal(t, "now()", Now(DialectPostgres))
	assert.Equal(t, "NOW()", Now(DialectMySQL))
	assert.Equal(t, "CURRENT_TIMESTAMP", Now(DialectSQLite))

	assert.Equal(t, "(now() + interval '15 minute')", AddInterval(DialectPostgres, Now(DialectPostgres), "15 minutes"))
	assert.Equal(t, "DATE_ADD(NOW(), INTERVAL 24 HOUR)", AddInterval(DialectMySQL, Now(DialectMySQL), "24 hours"))
	assert.Equal(t, "datetime(CURRENT_TIMESTAMP, '+1 hour')", AddInterval(DialectSQLite, Now(DialectSQLite), "1 hour"))

	clause, args := InList(DialectSQLite, "role", []string{"a", "b"})
	assert.Equal(t, "role IN (?, ?)", clause)
	assert.Equal(t, []any{"a", "b"}, args)

	assert.Equal(t, "ON CONFLICT (user_id, role) DO UPDATE SET status = EXCLUDED.status",
		UpsertClause(DialectPostgres, []string{"user_id", "role"}, []string{"status"}))
	assert.Equal(t, "ON CONFLICT (user_id, role) DO NOTHING",
		UpsertClause(DialectSQLite, []string{"user_id", "role"}, nil))
	assert.Equal(t, "ON DUPLICATE KEY UPDATE status = VALUES(status)",
		UpsertClause(DialectMySQL, []string{"user_id", "role"}, []string{"status"}))

	assert.Equal(t, "RETURNING id", ReturningClause(DialectPostgres, "id"))
	assert.Equal(t, "", ReturningClause(DialectMySQL, "id"))

	assert.NotEmpty(t, NewID())
}
