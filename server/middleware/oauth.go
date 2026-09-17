package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// OAuthConfig validates opaque Bearer tokens from a third-party OAuth
// provider via RFC 7662 token introspection. (For JWT-shaped tokens issued
// by an external OIDC provider, prefer jwt mode with `jwks_url` instead.)
type OAuthConfig struct {
	// IntrospectionURL is the RFC 7662 endpoint (https recommended).
	IntrospectionURL string
	// ClientID/Secret authenticate this service at the endpoint.
	ClientID     string
	ClientSecret string
	// CacheTTL caches active verdicts in memory (keyed by token hash).
	// Zero disables the cache. A cached token stays valid until TTL even
	// if revoked upstream — keep it short (default 60s).
	CacheTTL time.Duration
	// HTTPClient overrides the default client (5s timeout).
	HTTPClient *http.Client
}

type introspectCacheEntry struct {
	auth    AuthContext
	roles   []string
	expires time.Time
}

type introspectCache struct {
	mu      sync.Mutex
	entries map[string]introspectCacheEntry
}

func (c *introspectCache) get(key string) (AuthContext, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expires) {
		delete(c.entries, key)
		return AuthContext{}, false
	}
	out := e.auth
	out.Roles = append([]string(nil), e.roles...)
	return out, true
}

func (c *introspectCache) set(key string, auth AuthContext, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]introspectCacheEntry)
	}
	c.entries[key] = introspectCacheEntry{
		auth:    auth,
		roles:   append([]string(nil), auth.Roles...),
		expires: time.Now().Add(ttl),
	}
}

// Introspect validates a Bearer access token against the configured
// introspection endpoint and injects the resulting AuthContext (sub →
// UserID, scope words → Roles). Inactive/revoked tokens get 401.
func Introspect(cfg OAuthConfig) fiber.Handler {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	disabled := cfg.CacheTTL <= 0
	cache := &introspectCache{}
	return func(c fiber.Ctx) error {
		raw := c.Get("Authorization")
		token, ok := strings.CutPrefix(raw, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "missing bearer token",
			})
		}
		if cfg.IntrospectionURL == "" {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"code":    500,
				"message": "oauth introspection not configured",
			})
		}
		sum := sha256.Sum256([]byte(token))
		key := hex.EncodeToString(sum[:])
		if !disabled {
			if auth, hit := cache.get(key); hit {
				injectAuth(c, &auth)
				return c.Next()
			}
		}
		auth, err := introspectToken(c.Context(), client, cfg, token)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": err.Error(),
			})
		}
		if !disabled {
			cache.set(key, *auth, ttl)
		}
		injectAuth(c, auth)
		return c.Next()
	}
}

func introspectToken(ctx context.Context, client *http.Client, cfg OAuthConfig, token string) (*AuthContext, error) {
	form := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.IntrospectionURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("introspection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cfg.ClientID != "" {
		req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspection endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("introspection body: %w", err)
	}
	var claims struct {
		Active   bool   `json:"active"`
		Sub      string `json:"sub"`
		Username string `json:"username"`
		Scope    string `json:"scope"`
	}
	if err := json.Unmarshal(body, &claims); err != nil {
		return nil, fmt.Errorf("introspection decode: %w", err)
	}
	if !claims.Active {
		return nil, fmt.Errorf("token inactive or revoked")
	}
	user := claims.Sub
	if user == "" {
		user = claims.Username
	}
	if user == "" {
		return nil, fmt.Errorf("introspection response has no subject")
	}
	return &AuthContext{
		UserID:   user,
		RawToken: token,
		Roles:    strings.Fields(claims.Scope),
	}, nil
}
