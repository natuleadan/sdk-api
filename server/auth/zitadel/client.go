// Package zitadel provides Zitadel OIDC authentication and JWKS-based JWT validation.
package zitadel

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/natuleadan/sdk-api/server/auth/jwks"
)

// Client validates JWTs issued by Zitadel using JWKS.
type Client struct {
	issuer   string
	resolver *jwks.Resolver
}

// Config holds Zitadel connection settings.
type Config struct {
	Issuer string
	TTL    time.Duration
}

// NewClient creates a Zitadel JWKS client.
func NewClient(cfg Config) *Client {
	var opts []jwks.Option
	if cfg.TTL > 0 {
		opts = append(opts, jwks.WithTTL(cfg.TTL))
	}
	return &Client{
		issuer:   cfg.Issuer,
		resolver: jwks.NewWithDiscovery(cfg.Issuer, opts...),
	}
}

// ValidateToken validates a JWT token issued by Zitadel.
func (c *Client) ValidateToken(_ context.Context, tokenString string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(
		jwt.WithIssuer(c.issuer),
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}),
	)

	token, err := parser.Parse(tokenString, c.resolver.KeyFunc())
	if err != nil {
		return nil, fmt.Errorf("zitadel: token validation failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("zitadel: invalid token claims")
	}

	return claims, nil
}
