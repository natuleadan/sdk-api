package zitadel

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Client validates JWTs issued by Zitadel using JWKS.
type Client struct {
	issuer  string
	jwksURL string
	http    *http.Client
	keys    map[string]*rsa.PublicKey
	keysMu  sync.RWMutex
	keysExp time.Time
	ttl     time.Duration
}

// Config holds Zitadel connection settings.
type Config struct {
	Issuer string
	TTL    time.Duration
}

// NewClient creates a Zitadel JWKS client.
func NewClient(cfg Config) *Client {
	if cfg.TTL == 0 {
		cfg.TTL = 1 * time.Hour
	}

	jwksURL := cfg.Issuer + "/.well-known/openid-configuration"

	return &Client{
		issuer:  cfg.Issuer,
		jwksURL: jwksURL,
		http:    &http.Client{Timeout: 10 * time.Second},
		keys:    make(map[string]*rsa.PublicKey),
		ttl:     cfg.TTL,
	}
}

// ValidateToken validates a JWT token issued by Zitadel.
func (c *Client) ValidateToken(ctx context.Context, tokenString string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(
		jwt.WithIssuer(c.issuer),
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}),
	)

	token, err := parser.Parse(tokenString, c.keyFunc)
	if err != nil {
		return nil, fmt.Errorf("zitadel: token validation failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("zitadel: invalid token claims")
	}

	return claims, nil
}

// keyFunc is the callback for jwt.Parse to get the verification key.
func (c *Client) keyFunc(token *jwt.Token) (any, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("zitadel: token missing kid header")
	}

	keys, err := c.getKeys(context.Background())
	if err != nil {
		return nil, err
	}

	key, exists := keys[kid]
	if !exists {
		return nil, fmt.Errorf("zitadel: unknown kid %s", kid)
	}

	return key, nil
}

// getKeys fetches JWKS keys with caching.
func (c *Client) getKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	if keys, ok := c.keysFromCache(); ok {
		return keys, nil
	}

	c.keysMu.Lock()
	defer c.keysMu.Unlock()

	if time.Now().Before(c.keysExp) && len(c.keys) > 0 {
		return c.keys, nil
	}

	keys, err := c.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}

	c.keys = keys
	c.keysExp = time.Now().Add(c.ttl)

	return keys, nil
}

func (c *Client) keysFromCache() (map[string]*rsa.PublicKey, bool) {
	c.keysMu.RLock()
	defer c.keysMu.RUnlock()
	if time.Now().Before(c.keysExp) && len(c.keys) > 0 {
		keys := make(map[string]*rsa.PublicKey, len(c.keys))
		maps.Copy(keys, c.keys)
		return keys, true
	}
	return nil, false
}

func (c *Client) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	jwksURI, err := c.fetchOIDCConfig(ctx)
	if err != nil {
		return nil, err
	}

	jwksReq, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, fmt.Errorf("zitadel: failed to create JWKS request: %w", err)
	}
	jwksResp, err := c.http.Do(jwksReq)
	if err != nil {
		return nil, fmt.Errorf("zitadel: failed to fetch JWKS: %w", err)
	}
	defer func() {
		if err := jwksResp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "zitadel: jwks body close error: %v\n", err)
		}
	}()

	body, err := io.ReadAll(jwksResp.Body)
	if err != nil {
		return nil, fmt.Errorf("zitadel: failed to read JWKS: %w", err)
	}

	return parseJWKS(body)
}

func (c *Client) fetchOIDCConfig(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURL, nil)
	if err != nil {
		return "", fmt.Errorf("zitadel: failed to create JWKS config request: %w", err)
	}
	configResp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("zitadel: failed to fetch OIDC config: %w", err)
	}
	defer func() {
		if err := configResp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "zitadel: config body close error: %v\n", err)
		}
	}()

	var config struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(configResp.Body).Decode(&config); err != nil {
		return "", fmt.Errorf("zitadel: failed to parse OIDC config: %w", err)
	}
	return config.JWKSURI, nil
}

func parseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("zitadel: failed to parse JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" {
			continue
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			continue
		}

		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)

		keys[key.Kid] = &rsa.PublicKey{
			N: n,
			E: int(e.Int64()),
		}
	}

	return keys, nil
}
