package ory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Client wraps Ory Kratos (auth) and Keto (authorization).
type Client struct {
	kratosPublicURL string
	ketoURL         string
	http            *http.Client
}

// Config holds Ory connection settings.
type Config struct {
	KratosPublicURL string
	KetoURL         string
}

// NewClient creates an Ory client (Kratos + Keto).
func NewClient(cfg Config) *Client {
	return &Client{
		kratosPublicURL: cfg.KratosPublicURL,
		ketoURL:         cfg.KetoURL,
		http:            &http.Client{Timeout: 10 * time.Second},
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

// ValidateJWT validates a JWT signed by Ory Kratos.
func (c *Client) ValidateJWT(ctx context.Context, tokenString string, jwksURL string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
	)

	token, err := parser.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return nil, fmt.Errorf("ory: JWKS validation not yet implemented, use ValidateSession")
	})
	if err != nil {
		return nil, fmt.Errorf("ory: jwt validation failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("ory: invalid token claims")
	}

	return claims, nil
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
