// Package oauthstore provides a driver-agnostic fosite storage backed by
// database/sql. It works on PostgreSQL, MySQL/MariaDB and SQLite (Turso/
// libSQL): queries are written with $n placeholders and translated per
// dialect, timestamps are stored as UTC RFC3339 text (lexicographically
// ordered) and list columns as JSON text, so no driver-specific types leak in.
package oauthstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/ory/fosite"
)

// StringSlice stores a []string as a JSON text column so the same schema works
// on every driver.
type StringSlice []string

func (s *StringSlice) Scan(value any) error {
	if value == nil {
		*s = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("oauthstore: cannot scan %T into StringSlice", value)
	}
	if len(data) == 0 {
		*s = nil
		return nil
	}
	return json.Unmarshal(data, s)
}

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Store implements the fosite storage interfaces over database/sql.
type Store struct {
	db      *sql.DB
	dialect db.SQLDialect
}

// NewStore builds a store for the given YAML driver ("postgres", "mysql",
// "turso", "turso-serverless", "libsql", ...).
func NewStore(database *sql.DB, driver string) *Store {
	return &Store{db: database, dialect: db.DialectForDriver(driver)}
}

func nowRFC() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	q, a := db.Rewrite(s.dialect, query, args...)
	return s.db.ExecContext(ctx, q, a...)
}

func (s *Store) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	q, a := db.Rewrite(s.dialect, query, args...)
	return s.db.QueryRowContext(ctx, q, a...)
}

func (s *Store) getClient(id string) (fosite.Client, error) {
	var hashed []byte
	var redirectURIs, grantTypes, responseTypes, audience StringSlice
	var scopes string
	var isPublic bool
	err := s.queryRow(context.Background(),
		`SELECT hashed_secret, redirect_uris, grant_types, response_types, scopes, audience, is_public
		 FROM oauth_clients WHERE id = $1`, id).
		Scan(&hashed, &redirectURIs, &grantTypes, &responseTypes, &scopes, &audience, &isPublic)
	if err != nil {
		return nil, fosite.ErrNotFound
	}
	return &fosite.DefaultClient{
		ID:            id,
		Secret:        hashed,
		RedirectURIs:  redirectURIs,
		GrantTypes:    grantTypes,
		ResponseTypes: responseTypes,
		Scopes:        splitSpace(scopes),
		Audience:      audience,
		Public:        isPublic,
	}, nil
}

func (s *Store) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	return s.getClient(id)
}

func (s *Store) ClientAssertionJWTValid(ctx context.Context, jti string) error {
	var exists bool
	_ = s.queryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM oauth_jti_blacklist WHERE jti = $1 AND expires_at > $2)`,
		jti, nowRFC()).Scan(&exists)
	if exists {
		return fosite.ErrJTIKnown
	}
	return nil
}

func (s *Store) SetClientAssertionJWT(ctx context.Context, jti string, exp time.Time) error {
	clause := db.UpsertClause(s.dialect, []string{"jti"}, nil)
	_, err := s.exec(ctx,
		`INSERT INTO oauth_jti_blacklist (jti, expires_at) VALUES ($1, $2) `+clause,
		jti, exp.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) CreateAuthorizeCodeSession(ctx context.Context, code string, req fosite.Requester) error {
	return s.saveSession(ctx, code, "authorize_code", req)
}

func (s *Store) GetAuthorizeCodeSession(ctx context.Context, code string, _ fosite.Session) (fosite.Requester, error) {
	return s.loadSession(ctx, code, "authorize_code")
}

func (s *Store) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	_, err := s.exec(ctx, `UPDATE oauth_sessions SET active = false WHERE signature = $1 AND type = 'authorize_code'`, code)
	return err
}

func (s *Store) CreateAccessTokenSession(ctx context.Context, signature string, req fosite.Requester) error {
	return s.saveSession(ctx, signature, "access_token", req)
}

func (s *Store) GetAccessTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	return s.loadSession(ctx, signature, "access_token")
}

func (s *Store) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	_, err := s.exec(ctx, `DELETE FROM oauth_sessions WHERE signature = $1 AND type = 'access_token'`, signature)
	return err
}

func (s *Store) CreateRefreshTokenSession(ctx context.Context, signature, _ string, req fosite.Requester) error {
	return s.saveSession(ctx, signature, "refresh_token", req)
}

func (s *Store) GetRefreshTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	return s.loadSession(ctx, signature, "refresh_token")
}

func (s *Store) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	_, err := s.exec(ctx, `DELETE FROM oauth_sessions WHERE signature = $1 AND type = 'refresh_token'`, signature)
	return err
}

func (s *Store) RotateRefreshToken(ctx context.Context, requestID, refreshTokenSignature string) error {
	var oldSig string
	_ = s.queryRow(ctx,
		`SELECT signature FROM oauth_sessions WHERE request_id = $1 AND type = 'refresh_token' AND active = true ORDER BY created_at DESC LIMIT 1`,
		requestID).Scan(&oldSig)
	if oldSig != "" {
		_, _ = s.exec(ctx, `UPDATE oauth_sessions SET active = false WHERE signature = $1`, oldSig)
	}
	_, err := s.exec(ctx,
		`UPDATE oauth_sessions SET signature = $1 WHERE request_id = $2 AND type = 'refresh_token'`,
		refreshTokenSignature, requestID)
	return err
}

func (s *Store) RevokeRefreshToken(ctx context.Context, requestID string) error {
	_, err := s.exec(ctx, `UPDATE oauth_sessions SET active = false WHERE request_id = $1 AND type = 'refresh_token'`, requestID)
	return err
}

func (s *Store) RevokeAccessToken(ctx context.Context, requestID string) error {
	_, err := s.exec(ctx, `UPDATE oauth_sessions SET active = false WHERE request_id = $1 AND type = 'access_token'`, requestID)
	return err
}

func (s *Store) CreatePKCERequestSession(ctx context.Context, signature string, req fosite.Requester) error {
	return s.saveSession(ctx, signature, "pkce", req)
}

func (s *Store) GetPKCERequestSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	return s.loadSession(ctx, signature, "pkce")
}

func (s *Store) DeletePKCERequestSession(ctx context.Context, signature string) error {
	_, err := s.exec(ctx, `DELETE FROM oauth_sessions WHERE signature = $1 AND type = 'pkce'`, signature)
	return err
}

func (s *Store) saveSession(ctx context.Context, signature, stype string, req fosite.Requester) error {
	sessData, _ := json.Marshal(req.GetSession())
	formData, _ := json.Marshal(req.GetRequestForm())
	clause := db.UpsertClause(s.dialect, []string{"signature", "type"}, []string{"session_data", "active"})
	_, err := s.exec(ctx,
		`INSERT INTO oauth_sessions
		 (signature, type, request_id, client_id, requested_scopes, granted_scopes,
		  requested_audience, granted_audience, session_data, form, lang, active, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,true,$12,$13) `+clause,
		signature, stype, req.GetID(), req.GetClient().GetID(),
		StringSlice(req.GetRequestedScopes()), StringSlice(req.GetGrantedScopes()),
		StringSlice(req.GetRequestedAudience()), StringSlice(req.GetGrantedAudience()),
		sessData, formData, "",
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339), nowRFC())
	return err
}

func (s *Store) loadSession(ctx context.Context, signature, stype string) (fosite.Requester, error) {
	var requestID, clientID string
	var requestedScope, grantedScope, requestedAudience, grantedAudience StringSlice
	var sessData, formData []byte
	var active bool
	err := s.queryRow(ctx,
		`SELECT request_id, client_id, requested_scopes, granted_scopes,
		        requested_audience, granted_audience, session_data, form, active
		 FROM oauth_sessions WHERE signature = $1 AND type = $2`,
		signature, stype).Scan(
		&requestID, &clientID, &requestedScope, &grantedScope,
		&requestedAudience, &grantedAudience, &sessData, &formData, &active)
	if err != nil {
		return nil, fosite.ErrNotFound
	}
	if !active {
		return nil, fosite.ErrInactiveToken
	}
	client, err := s.getClient(clientID)
	if err != nil {
		return nil, fosite.ErrNotFound
	}
	var sess fosite.DefaultSession
	if len(sessData) > 0 {
		_ = json.Unmarshal(sessData, &sess)
	}
	var form url.Values
	if len(formData) > 0 {
		_ = json.Unmarshal(formData, &form)
	}
	return &fosite.Request{
		ID:                requestID,
		RequestedAt:       time.Now(),
		Client:            client,
		RequestedScope:    fosite.Arguments(requestedScope),
		GrantedScope:      fosite.Arguments(grantedScope),
		RequestedAudience: fosite.Arguments(requestedAudience),
		GrantedAudience:   fosite.Arguments(grantedAudience),
		Session:           &sess,
		Form:              form,
	}, nil
}

func (s *Store) Authenticate(ctx context.Context, clientID, secret string) (string, error) {
	client, err := s.getClient(clientID)
	if err != nil {
		return "", fosite.ErrNotFound
	}
	if !auth.VerifyPassword(string(client.GetHashedSecret()), secret) {
		return "", fosite.ErrInvalidClient
	}
	return clientID, nil
}

func (s *Store) GetPublicKey(context.Context, string, string, string) (*jose.JSONWebKey, error) {
	return nil, fosite.ErrNotFound
}

func (s *Store) GetPublicKeys(context.Context, string, string) (*jose.JSONWebKeySet, error) {
	return nil, fosite.ErrNotFound
}

func (s *Store) GetPublicKeyScopes(context.Context, string, string, string) ([]string, error) {
	return nil, fosite.ErrNotFound
}

func (s *Store) IsJWTUsed(context.Context, string) (bool, error) { return false, nil }

func (s *Store) MarkJWTUsedForTime(context.Context, string, time.Time) error { return nil }

func (s *Store) CreateOpenIDConnectSession(ctx context.Context, authorizeCode string, requester fosite.Requester) error {
	sessData, _ := json.Marshal(requester.GetSession())
	clause := db.UpsertClause(s.dialect, []string{"signature", "type"}, []string{"session_data"})
	_, err := s.exec(ctx,
		`INSERT INTO oauth_sessions (signature, type, request_id, client_id, session_data, active, expires_at, created_at)
		 VALUES ($1, 'oidc', $2, $3, $4, true, $5, $6) `+clause,
		authorizeCode, requester.GetID(), requester.GetClient().GetID(), sessData,
		time.Now().UTC().Add(10*time.Minute).Format(time.RFC3339), nowRFC())
	return err
}

func (s *Store) GetOpenIDConnectSession(ctx context.Context, authorizeCode string, _ fosite.Requester) (fosite.Requester, error) {
	var requestID, clientID string
	var sessData []byte
	err := s.queryRow(ctx,
		`SELECT request_id, client_id, session_data FROM oauth_sessions WHERE signature = $1 AND type = 'oidc' AND active = true`,
		authorizeCode).Scan(&requestID, &clientID, &sessData)
	if err != nil {
		return nil, fosite.ErrNotFound
	}
	client, err := s.getClient(clientID)
	if err != nil {
		return nil, fosite.ErrNotFound
	}
	var sess fosite.DefaultSession
	_ = json.Unmarshal(sessData, &sess)
	return &fosite.Request{ID: requestID, Client: client, Session: &sess}, nil
}

func (s *Store) DeleteOpenIDConnectSession(ctx context.Context, authorizeCode string) error {
	_, err := s.exec(ctx, `DELETE FROM oauth_sessions WHERE signature = $1 AND type = 'oidc'`, authorizeCode)
	return err
}

func splitSpace(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}

var _ fosite.ClientManager = (*Store)(nil)
