// Package jwks provides JWKS key resolution for JWT signature validation.
// It fetches RSA public keys from a JWKS endpoint, caches them by kid, and
// exposes a jwt.Keyfunc for use with the JWT middleware and auth clients.
package jwks

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

// Resolver fetches and caches JWKS RSA keys by kid.
type Resolver struct {
	jwksURL string
	http    *http.Client
	keys    map[string]*rsa.PublicKey
	keysMu  sync.RWMutex
	keysExp time.Time
	ttl     time.Duration
}

// Option configures a Resolver.
type Option func(*Resolver)

// WithHTTPClient sets the HTTP client used for JWKS fetches.
func WithHTTPClient(c *http.Client) Option {
	return func(r *Resolver) { r.http = c }
}

// WithTTL sets the JWKS cache TTL (default 1h).
func WithTTL(ttl time.Duration) Option {
	return func(r *Resolver) { r.ttl = ttl }
}

// New creates a Resolver for a direct JWKS URL (e.g. https://host/.well-known/jwks.json).
func New(jwksURL string, opts ...Option) *Resolver {
	return newResolver(jwksURL, opts...)
}

// NewWithDiscovery creates a Resolver that first fetches the OIDC discovery
// document to locate the jwks_uri. issuer is e.g. https://host/.
func NewWithDiscovery(issuer string, opts ...Option) *Resolver {
	return newResolver(issuer+"/.well-known/openid-configuration", opts...)
}

func newResolver(jwksURL string, opts ...Option) *Resolver {
	r := &Resolver{
		jwksURL: jwksURL,
		http:    &http.Client{Timeout: 10 * time.Second},
		keys:    make(map[string]*rsa.PublicKey),
		ttl:     time.Hour,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// KeyFunc returns a jwt.Keyfunc that resolves the verification key by kid.
func (r *Resolver) KeyFunc() jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("jwks: token missing kid header")
		}
		keys, err := r.getKeys(context.Background())
		if err != nil {
			return nil, err
		}
		key, exists := keys[kid]
		if !exists {
			return nil, fmt.Errorf("jwks: unknown kid %s", kid)
		}
		return key, nil
	}
}

// getKeys fetches JWKS keys with caching.
func (r *Resolver) getKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	if keys, ok := r.keysFromCache(); ok {
		return keys, nil
	}

	r.keysMu.Lock()
	defer r.keysMu.Unlock()

	if time.Now().Before(r.keysExp) && len(r.keys) > 0 {
		return r.keys, nil
	}

	keys, err := r.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}

	r.keys = keys
	r.keysExp = time.Now().Add(r.ttl)

	return keys, nil
}

func (r *Resolver) keysFromCache() (map[string]*rsa.PublicKey, bool) {
	r.keysMu.RLock()
	defer r.keysMu.RUnlock()
	if time.Now().Before(r.keysExp) && len(r.keys) > 0 {
		keys := make(map[string]*rsa.PublicKey, len(r.keys))
		maps.Copy(keys, r.keys)
		return keys, true
	}
	return nil, false
}

func (r *Resolver) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	body, err := r.fetch(ctx, r.jwksURL)
	if err != nil {
		return nil, fmt.Errorf("jwks: fetch: %w", err)
	}

	var config struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("jwks: parse response: %w", err)
	}
	if config.JWKSURI != "" {
		body, err = r.fetch(ctx, config.JWKSURI)
		if err != nil {
			return nil, fmt.Errorf("jwks: fetch jwks_uri: %w", err)
		}
	}

	return ParseJWKS(body)
}

func (r *Resolver) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "jwks: body close error: %v\n", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks: %s returned %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ParseJWKS parses a JWKS document into a kid-keyed map of RSA public keys.
func ParseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("jwks: parse: %w", err)
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
		keys[key.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}

	return keys, nil
}
