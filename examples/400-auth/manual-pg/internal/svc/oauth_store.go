package svc

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/fosite"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/language"
	"github.com/go-jose/go-jose/v3"
)

type oauthSession struct {
	ID                string
	RequestedAt       time.Time
	ClientID          string
	RequestedScope    fosite.Arguments
	GrantedScope      fosite.Arguments
	RequestedAudience fosite.Arguments
	GrantedAudience   fosite.Arguments
	Session           json.RawMessage
	Form              url.Values
	Lang              string
	Active            bool
}

func (s *oauthSession) toRequester(client fosite.Client) *fosite.AuthorizeRequest {
	var sess fosite.DefaultSession
	if len(s.Session) > 0 {
		json.Unmarshal(s.Session, &sess)
	}
	tag, _ := language.Parse(s.Lang)
	return &fosite.AuthorizeRequest{
		ResponseTypes:        nil,
		RedirectURI:          nil,
		State:                "",
		HandledResponseTypes: nil,
		Request: fosite.Request{
			ID:                s.ID,
			RequestedAt:       s.RequestedAt,
			Client:            client,
			RequestedScope:    s.RequestedScope,
			GrantedScope:      s.GrantedScope,
			RequestedAudience: s.RequestedAudience,
			GrantedAudience:   s.GrantedAudience,
			Session:           &sess,
			Form:              s.Form,
			Lang:              tag,
		},
	}
}

type OAuthStore struct {
	pool *pgxpool.Pool
}

func NewOAuthStore(pool *pgxpool.Pool) *OAuthStore {
	return &OAuthStore{pool: pool}
}

func (s *OAuthStore) getClient(id string) (fosite.Client, error) {
	var hashed []byte
	var redirectURIs, grantTypes, responseTypes, audience []string
	var scopes string
	var isPublic bool
	err := s.pool.QueryRow(context.Background(),
		`SELECT hashed_secret, redirect_uris, grant_types, response_types, scopes, audience, is_public
		 FROM oauth_clients WHERE id = $1`, id).
		Scan(&hashed, &redirectURIs, &grantTypes, &responseTypes, &scopes, &audience, &isPublic)
	if err != nil {
		return nil, fosite.ErrNotFound
	}
	return &fosite.DefaultClient{
		ID:             id,
		Secret:         hashed,
		RedirectURIs:   redirectURIs,
		GrantTypes:     grantTypes,
		ResponseTypes:  responseTypes,
		Scopes:         stringsSplitSpace(scopes),
		Audience:       audience,
		Public:         isPublic,
	}, nil
}

func (s *OAuthStore) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	return s.getClient(id)
}

func (s *OAuthStore) ClientAssertionJWTValid(ctx context.Context, jti string) error {
	var exists bool
	s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM oauth_jti_blacklist WHERE jti = $1 AND expires_at > now())`, jti).Scan(&exists)
	if exists {
		return fosite.ErrJTIKnown
	}
	return nil
}

func (s *OAuthStore) SetClientAssertionJWT(ctx context.Context, jti string, exp time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO oauth_jti_blacklist (jti, expires_at) VALUES ($1, $2) ON CONFLICT DO NOTHING`, jti, exp)
	return err
}

func (s *OAuthStore) CreateAuthorizeCodeSession(ctx context.Context, code string, req fosite.Requester) error {
	return s.saveSession(ctx, code, "authorize_code", req)
}

func (s *OAuthStore) GetAuthorizeCodeSession(ctx context.Context, code string, _ fosite.Session) (fosite.Requester, error) {
	return s.loadSession(ctx, code, "authorize_code")
}

func (s *OAuthStore) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	_, err := s.pool.Exec(ctx, `UPDATE oauth_sessions SET active = false WHERE signature = $1 AND type = 'authorize_code'`, code)
	return err
}

func (s *OAuthStore) CreateAccessTokenSession(ctx context.Context, signature string, req fosite.Requester) error {
	return s.saveSession(ctx, signature, "access_token", req)
}

func (s *OAuthStore) GetAccessTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	return s.loadSession(ctx, signature, "access_token")
}

func (s *OAuthStore) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_sessions WHERE signature = $1 AND type = 'access_token'`, signature)
	return err
}

func (s *OAuthStore) CreateRefreshTokenSession(ctx context.Context, signature, accessSig string, req fosite.Requester) error {
	_ = accessSig
	return s.saveSession(ctx, signature, "refresh_token", req)
}

func (s *OAuthStore) GetRefreshTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	return s.loadSession(ctx, signature, "refresh_token")
}

func (s *OAuthStore) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_sessions WHERE signature = $1 AND type = 'refresh_token'`, signature)
	return err
}

func (s *OAuthStore) RotateRefreshToken(ctx context.Context, requestID, refreshTokenSignature string) error {
	var oldSig string
	s.pool.QueryRow(ctx,
		`SELECT signature FROM oauth_sessions WHERE request_id = $1 AND type = 'refresh_token' AND active = true ORDER BY created_at DESC LIMIT 1`,
		requestID).Scan(&oldSig)
	if oldSig != "" {
		s.pool.Exec(ctx, `UPDATE oauth_sessions SET active = false WHERE signature = $1`, oldSig)
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE oauth_sessions SET signature = $1 WHERE request_id = $2 AND type = 'refresh_token'`,
		refreshTokenSignature, requestID)
	return err
}

func (s *OAuthStore) RevokeRefreshToken(ctx context.Context, requestID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE oauth_sessions SET active = false WHERE request_id = $1 AND type = 'refresh_token'`, requestID)
	return err
}

func (s *OAuthStore) RevokeAccessToken(ctx context.Context, requestID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE oauth_sessions SET active = false WHERE request_id = $1 AND type = 'access_token'`, requestID)
	return err
}

func (s *OAuthStore) CreatePKCERequestSession(ctx context.Context, signature string, req fosite.Requester) error {
	return s.saveSession(ctx, signature, "pkce", req)
}

func (s *OAuthStore) GetPKCERequestSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	return s.loadSession(ctx, signature, "pkce")
}

func (s *OAuthStore) DeletePKCERequestSession(ctx context.Context, signature string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_sessions WHERE signature = $1 AND type = 'pkce'`, signature)
	return err
}

func (s *OAuthStore) saveSession(ctx context.Context, signature, stype string, req fosite.Requester) error {
	sessData, _ := json.Marshal(req.GetSession())
	form := req.GetRequestForm()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oauth_sessions
		 (signature, type, request_id, client_id, requested_scopes, granted_scopes,
		  requested_audience, granted_audience, session_data, form, lang, active, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,true,$12)
		 ON CONFLICT (signature, type) DO UPDATE SET session_data = EXCLUDED.session_data, active = true`,
		signature, stype, req.GetID(), req.GetClient().GetID(),
		req.GetRequestedScopes(), req.GetGrantedScopes(),
		req.GetRequestedAudience(), req.GetGrantedAudience(),
		sessData, form, "", time.Now().Add(time.Hour))
	return err
}

type rawRequester struct {
	ID                string
	ClientID          string
	RequestedScope    fosite.Arguments
	GrantedScope      fosite.Arguments
	RequestedAudience fosite.Arguments
	GrantedAudience   fosite.Arguments
	Session           json.RawMessage
	Form              url.Values
	Active            bool
}

func (s *OAuthStore) loadSession(ctx context.Context, signature, stype string) (fosite.Requester, error) {
	var raw rawRequester
	var sessData, formData []byte
	err := s.pool.QueryRow(ctx,
		`SELECT request_id, client_id, requested_scopes, granted_scopes,
		        requested_audience, granted_audience, session_data, form, active
		 FROM oauth_sessions WHERE signature = $1 AND type = $2`,
		signature, stype).Scan(
		&raw.ID, &raw.ClientID, &raw.RequestedScope, &raw.GrantedScope,
		&raw.RequestedAudience, &raw.GrantedAudience, &sessData, &formData, &raw.Active)
	if err != nil {
		return nil, fosite.ErrNotFound
	}
	raw.Session = sessData
	json.Unmarshal(formData, &raw.Form)

	if !raw.Active {
		return nil, fosite.ErrInactiveToken
	}

	client, err := s.getClient(raw.ClientID)
	if err != nil {
		return nil, fosite.ErrNotFound
	}

	var sess fosite.DefaultSession
	if len(raw.Session) > 0 {
		json.Unmarshal(raw.Session, &sess)
	}

	return &fosite.Request{
		ID:                raw.ID,
		RequestedAt:       time.Now(),
		Client:            client,
		RequestedScope:    raw.RequestedScope,
		GrantedScope:      raw.GrantedScope,
		RequestedAudience: raw.RequestedAudience,
		GrantedAudience:   raw.GrantedAudience,
		Session:           &sess,
		Form:              raw.Form,
	}, nil
}

func stringsSplitSpace(s string) []string {
	if s == "" {
		return nil
	}
	var r []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' {
			if i > start {
				r = append(r, s[start:i])
			}
			start = i + 1
		}
	}
	return r
}

func (s *OAuthStore) Authenticate(ctx context.Context, clientID string, secret string) (string, error) {
	client, err := s.getClient(clientID)
	if err != nil {
		return "", fosite.ErrNotFound
	}
	if err := bcrypt.CompareHashAndPassword(client.GetHashedSecret(), []byte(secret)); err != nil {
		return "", fosite.ErrInvalidClient
	}
	return clientID, nil
}

func (s *OAuthStore) GetPublicKey(ctx context.Context, issuer string, subject string, keyId string) (*jose.JSONWebKey, error) {
	return nil, fosite.ErrNotFound
}

func (s *OAuthStore) GetPublicKeys(ctx context.Context, issuer string, subject string) (*jose.JSONWebKeySet, error) {
	return nil, fosite.ErrNotFound
}

func (s *OAuthStore) GetPublicKeyScopes(ctx context.Context, issuer string, subject string, keyId string) ([]string, error) {
	return nil, fosite.ErrNotFound
}

func (s *OAuthStore) IsJWTUsed(ctx context.Context, jti string) (bool, error) {
	return false, nil
}

func (s *OAuthStore) MarkJWTUsedForTime(ctx context.Context, jti string, exp time.Time) error {
	return nil
}

func (s *OAuthStore) CreateOpenIDConnectSession(ctx context.Context, authorizeCode string, requester fosite.Requester) error {
	sessData, _ := json.Marshal(requester.GetSession())
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oauth_sessions (signature, type, request_id, client_id, session_data, active, expires_at)
		 VALUES ($1, 'oidc', $2, $3, $4, true, now() + interval '10 minutes')
		 ON CONFLICT (signature, type) DO UPDATE SET session_data = EXCLUDED.session_data`,
		authorizeCode, requester.GetID(), requester.GetClient().GetID(), sessData)
	return err
}

func (s *OAuthStore) GetOpenIDConnectSession(ctx context.Context, authorizeCode string, requester fosite.Requester) (fosite.Requester, error) {
	var sessData []byte
	var clientID, requestID string
	err := s.pool.QueryRow(ctx,
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
	json.Unmarshal(sessData, &sess)
	return &fosite.Request{
		ID:      requestID,
		Client:  client,
		Session: &sess,
	}, nil
}

func (s *OAuthStore) DeleteOpenIDConnectSession(ctx context.Context, authorizeCode string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_sessions WHERE signature = $1 AND type = 'oidc'`, authorizeCode)
	return err
}
