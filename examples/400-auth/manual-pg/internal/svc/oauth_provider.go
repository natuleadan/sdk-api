package svc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
)

type OAuthProvider struct {
	Provider fosite.OAuth2Provider
	Store    *OAuthStore
}

func NewOAuthProvider(pool *pgxpool.Pool, jwtSecret string) *OAuthProvider {
	store := NewOAuthStore(pool)

	config := &fosite.Config{
		GlobalSecret:                []byte(ensure32(jwtSecret)),
		AccessTokenLifespan:         time.Hour,
		RefreshTokenLifespan:        30 * 24 * time.Hour,
		AuthorizeCodeLifespan:       10 * time.Minute,
		EnforcePKCE:                 true,
		EnforcePKCEForPublicClients: true,
		ScopeStrategy:               fosite.HierarchicScopeStrategy,
		SendDebugMessagesToClients:  true,
	}

	strategy := compose.NewOAuth2HMACStrategy(config)

	provider := compose.Compose(config, store, strategy,
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2PKCEFactory,
		compose.OAuth2ClientCredentialsGrantFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2TokenRevocationFactory,
		compose.OAuth2TokenIntrospectionFactory,
	)

	return &OAuthProvider{
		Provider: provider,
		Store:    store,
	}
}

func (p *OAuthProvider) GetSession(subject, orgID string) *fosite.DefaultSession {
	session := &fosite.DefaultSession{Subject: subject}
	if orgID != "" {
		session.Extra = map[string]any{"org_id": orgID}
	}
	return session
}

func ensure32(secret string) string {
	if len(secret) >= 32 {
		return secret[:32]
	}
	b := make([]byte, 32-len(secret))
	rand.Read(b)
	return secret + hex.EncodeToString(b)
}

func init() {
	_ = context.Background
}
