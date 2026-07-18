// Package ory provides Ory Kratos authentication and Keto authorization client.
package ory

import (
	"bytes"
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

// Client wraps Ory Kratos (auth) and Keto (authorization).
type Client struct {
	kratosPublicURL string
	ketoURL         string
	http            *http.Client
	jwksURL         string
	keys            map[string]*rsa.PublicKey
	keysMu          sync.RWMutex
	keysExp         time.Time
	ttl             time.Duration
}

// Config holds Ory connection settings.
type Config struct {
	KratosPublicURL string
	KetoURL         string
	TTL             time.Duration
}

// NewClient creates an Ory client (Kratos + Keto).
func NewClient(cfg Config) *Client {
	if cfg.TTL == 0 {
		cfg.TTL = 1 * time.Hour
	}
	return &Client{
		kratosPublicURL: cfg.KratosPublicURL,
		ketoURL:         cfg.KetoURL,
		http:            &http.Client{Timeout: 10 * time.Second},
		jwksURL:         cfg.KratosPublicURL + "/.well-known/jwks.json",
		keys:            make(map[string]*rsa.PublicKey),
		ttl:             cfg.TTL,
	}
}

// Session holds the validated user session from Kratos.
type Session struct {
	Identity struct {
		ID     string         `json:"id"`
		Traits map[string]any `json:"traits"`
	} `json:"identity"`
}

// ValidateSession validates a session cookie or token against Ory Kratos.
func (c *Client) ValidateSession(ctx context.Context, token string) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.kratosPublicURL+"/sessions/whoami", nil)
	if err != nil {
		return nil, fmt.Errorf("ory: failed to create whoami request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ory: whoami request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "ory: whoami body close error: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ory: whoami returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ory: failed to read whoami response: %w", err)
	}

	var session Session
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("ory: failed to parse whoami response: %w", err)
	}

	return &session, nil
}

// ValidateJWT validates a JWT signed by Ory Kratos using JWKS.
func (c *Client) ValidateJWT(_ context.Context, tokenString string, _ string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
	)

	token, err := parser.Parse(tokenString, c.keyFunc)
	if err != nil {
		return nil, fmt.Errorf("ory: jwt validation failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("ory: invalid token claims")
	}

	return claims, nil
}

// keyFunc is the callback for jwt.Parse to get the verification key.
func (c *Client) keyFunc(token *jwt.Token) (any, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("ory: token missing kid header")
	}

	keys, err := c.getKeys(context.Background())
	if err != nil {
		return nil, err
	}

	key, exists := keys[kid]
	if !exists {
		return nil, fmt.Errorf("ory: unknown kid %s", kid)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ory: failed to create JWKS request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ory: failed to fetch JWKS: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "ory: jwks body close error: %v\n", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ory: failed to read JWKS: %w", err)
	}

	return parseJWKS(body)
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
		return nil, fmt.Errorf("ory: failed to parse JWKS: %w", err)
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

// KetoCheckRequest defines an authorization check against Ory Keto.
type KetoCheckRequest struct {
	Namespace string
	Object    string
	Relation  string
	SubjectID string
}

// KetoCheck performs an authorization check against Ory Keto.
func (c *Client) KetoCheck(ctx context.Context, req KetoCheckRequest) (bool, error) {
	body, err := json.Marshal(map[string]any{
		"namespace":  req.Namespace,
		"object":     req.Object,
		"relation":   req.Relation,
		"subject_id": req.SubjectID,
	})
	if err != nil {
		return false, fmt.Errorf("ory: keto check marshal failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.ketoURL+"/relation-tuples/check", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("ory: keto check request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("ory: keto check failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "ory: keto body close error: %v\n", err)
		}
	}()

	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("ory: failed to parse keto check response: %w", err)
	}

	return result.Allowed, nil
}

// WriteKetoTuple writes a relation tuple to Ory Keto.
func (c *Client) WriteKetoTuple(ctx context.Context, namespace, object, relation, subjectID string) error {
	body, err := json.Marshal(map[string]any{
		"namespace":  namespace,
		"object":     object,
		"relation":   relation,
		"subject_id": subjectID,
	})
	if err != nil {
		return fmt.Errorf("ory: keto write marshal failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.ketoURL+"/admin/relation-tuples", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ory: keto write request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ory: keto write tuple failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(io.Discard, "ory: keto write close error: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ory: keto write tuple returned %d", resp.StatusCode)
	}

	return nil
}
