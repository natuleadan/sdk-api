package auth

import (
	"context"
	"time"

	"auth-roles/internal/svc"
	"github.com/natuleadan/sdk-api/db"
)

// SQLStore is the Store implementation over the example's driver-agnostic
// Querier. Queries are written in PostgreSQL syntax and rewritten per dialect.
type SQLStore struct {
	Q *svc.Querier
}

// NewSQLStore wires the store to a Querier.
func NewSQLStore(q *svc.Querier) *SQLStore { return &SQLStore{Q: q} }

// EnsureSchema creates the grants tables and teams table (idempotent). The id
// and reference columns use VARCHAR on MySQL, where a TEXT primary key needs a
// key length; PG/Turso accept TEXT.
func (s *SQLStore) EnsureSchema(ctx context.Context) error {
	strType := "TEXT"
	if s.Q.Dialect() == db.DialectMySQL {
		strType = "VARCHAR(255)"
	}
	if _, err := s.Q.Exec(ctx, `
CREATE TABLE IF NOT EXISTS auth_grants (
    id `+strType+` PRIMARY KEY,
    grantor `+strType+` NOT NULL,
    grantee `+strType+` NOT NULL,
    permission `+strType+` NOT NULL,
    resource `+strType+` NOT NULL DEFAULT '*',
    status `+strType+` NOT NULL DEFAULT 'active',
    expires_at TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
)`); err != nil {
		return err
	}
	_, err := s.Q.Exec(ctx, `
CREATE TABLE IF NOT EXISTS auth_teams (
    id `+strType+` PRIMARY KEY,
    name `+strType+` NOT NULL,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
)`)
	return err
}

func (s *SQLStore) InsertGrant(grantor, grantee, permission, resource string, expiresAt *time.Time) (string, error) {
	if resource == "" {
		resource = "*"
	}
	var exp any
	if expiresAt != nil {
		exp = expiresAt.UTC().Format(time.RFC3339)
	}
	id := db.NewID()
	_, err := s.Q.Exec(context.Background(),
		`INSERT INTO auth_grants (id, grantor, grantee, permission, resource, status, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,'active',$6, now())`,
		id, grantor, grantee, permission, resource, exp)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *SQLStore) ActiveGrants(grantee string) ([]Grant, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.Q.Query(context.Background(),
		`SELECT id, grantor, grantee, permission, resource, status, created_at
		 FROM auth_grants
		 WHERE grantee = $1 AND status = 'active'
		   AND (expires_at IS NULL OR expires_at > $2)`, grantee, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.Grantor, &g.Grantee, &g.Permission, &g.Resource, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *SQLStore) RevokeGrant(id, grantor string) (bool, error) {
	tag, err := s.Q.Exec(context.Background(),
		`UPDATE auth_grants SET status = 'revoked' WHERE id = $1 AND grantor = $2 AND status = 'active'`, id, grantor)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *SQLStore) ListGrants(user string) ([]Grant, error) {
	rows, err := s.Q.Query(context.Background(),
		`SELECT id, grantor, grantee, permission, resource, status, created_at
		 FROM auth_grants WHERE grantor = $1 OR grantee = $1 ORDER BY created_at DESC`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.Grantor, &g.Grantee, &g.Permission, &g.Resource, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

var _ Store = (*SQLStore)(nil)
