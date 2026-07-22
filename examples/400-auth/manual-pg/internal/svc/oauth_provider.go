package svc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"log"
	"os"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"
)

type OAuthProvider struct {
	Provider   fosite.OAuth2Provider
	Store      *OAuthStore
	PrivateKey *rsa.PrivateKey
	JWKS       *jose.JSONWebKeySet
}

func loadOrFailPrivateKey() *rsa.PrivateKey {
	b64Data := os.Getenv("OIDC_PRIVATE_KEY_B64")
	if b64Data == "" {
		log.Fatal("[OIDC] OIDC_PRIVATE_KEY_B64 env var is required")
	}
	pemBytes, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		log.Fatalf("[OIDC] OIDC_PRIVATE_KEY_B64: %v", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		log.Fatal("[OIDC] OIDC_PRIVATE_KEY: invalid PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		log.Fatalf("[OIDC] parse private key: %v", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		log.Fatal("[OIDC] OIDC_PRIVATE_KEY must be an RSA private key")
	}
	return rsaKey
}

func buildJWKS(key *rsa.PrivateKey) *jose.JSONWebKeySet {
	pub := jose.JSONWebKey{
		Key:       &key.PublicKey,
		KeyID:     "1",
		Algorithm: "RS256",
		Use:       "sig",
	}
	return &jose.JSONWebKeySet{Keys: []jose.JSONWebKey{pub}}
}

func NewOAuthProvider(pool *pgxpool.Pool, jwtSecret string) *OAuthProvider {
	store := NewOAuthStore(pool)

	privateKey := loadOrFailPrivateKey()
	jwks := buildJWKS(privateKey)

	config := &fosite.Config{
		GlobalSecret:                []byte(ensure32(jwtSecret)),
		AccessTokenLifespan:         time.Hour,
		RefreshTokenLifespan:        30 * 24 * time.Hour,
		AuthorizeCodeLifespan:       10 * time.Minute,
		IDTokenLifespan:             time.Hour,
		EnforcePKCE:                 true,
		EnforcePKCEForPublicClients: true,
		ScopeStrategy:               fosite.HierarchicScopeStrategy,
		SendDebugMessagesToClients:  true,
		IDTokenIssuer:               "http://localhost:23400/",
	}

	provider := compose.ComposeAllEnabled(config, store, privateKey)

	return &OAuthProvider{
		Provider:   provider,
		Store:      store,
		PrivateKey: privateKey,
		JWKS:       jwks,
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
