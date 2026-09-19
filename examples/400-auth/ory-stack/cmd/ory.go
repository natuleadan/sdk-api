package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// oryBootstrap holds the identifiers the tests need: the Kratos identity used
// to log in and the Keto endpoint.
type oryBootstrap struct {
	KratosURL string `json:"kratos_url"`
	KetoURL   string `json:"keto_url"`
	Identity  string `json:"identity"`
	Password  string `json:"password"`
}

// bootstrapOry creates (idempotently) a Kratos identity with a password so the
// example needs no manual Ory setup. It returns the identifiers the tests use.
func bootstrapOry(kratosAdminURL, email, password string) (*oryBootstrap, error) {
	if kratosAdminURL == "" {
		kratosAdminURL = "http://localhost:14434"
	}
	kratosAdminURL = strings.TrimRight(kratosAdminURL, "/")

	// Ensure the identity exists. Reusing it (instead of recreating) keeps the
	// Kratos identity id stable across restarts, so previously issued sessions
	// and Keto tuples stay valid.
	if id := findIdentity(kratosAdminURL, email); id != "" {
		return &oryBootstrap{
			KratosURL: kratosAdminURL,
			Identity:  email,
			Password:  password,
		}, nil
	}

	body := map[string]any{
		"schema_id": "default",
		"traits":    map[string]any{"email": email, "org_id": "org-alfa"},
		"credentials": map[string]any{
			"password": map[string]any{"config": map[string]any{"password": password}},
		},
		"state": "active",
	}
	st, resp, err := kratosCall(kratosAdminURL, http.MethodPost, "/admin/identities", body)
	if err != nil {
		return nil, err
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp), &created); err != nil || created.ID == "" {
		return nil, fmt.Errorf("create identity (%d): %s", st, resp)
	}

	return &oryBootstrap{
		KratosURL: kratosAdminURL,
		Identity:  email,
		Password:  password,
	}, nil
}

// findIdentity returns the id of an identity with the given email, if any.
func findIdentity(adminURL, email string) string {
	st, body, err := kratosCall(adminURL, http.MethodGet, "/admin/identities?page_size=250", nil)
	if err != nil || st != http.StatusOK {
		return ""
	}
	var list []struct {
		ID     string `json:"id"`
		Traits struct {
			Email string `json:"email"`
		} `json:"traits"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		return ""
	}
	for _, id := range list {
		if id.Traits.Email == email {
			return id.ID
		}
	}
	return ""
}

// kratosLogin performs an API login flow and returns the session token.
func kratosLogin(publicURL, email, password string) (string, error) {
	if publicURL == "" {
		publicURL = "http://localhost:14433"
	}
	publicURL = strings.TrimRight(publicURL, "/")

	st, body, err := kratosCall(publicURL, http.MethodGet, "/self-service/login/api", nil)
	if err != nil {
		return "", err
	}
	var flow struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &flow); err != nil || flow.ID == "" {
		return "", fmt.Errorf("create login flow (%d): %s", st, body)
	}

	submit := map[string]any{
		"method":     "password",
		"identifier": email,
		"password":   password,
	}
	st, body, err = kratosCall(publicURL, http.MethodPost, "/self-service/login?flow="+flow.ID, submit)
	if err != nil {
		return "", err
	}
	var result struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil || result.SessionToken == "" {
		return "", fmt.Errorf("submit login (%d): %s", st, body)
	}
	return result.SessionToken, nil
}

func kratosCall(base, method, path string, payload any) (int, string, error) {
	var r io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, r)
	if err != nil {
		return 0, "", err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

// writeOryBootstrap persists the identifiers for the test process.
func writeOryBootstrap(path string, info *oryBootstrap) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

// ensureKetoReady waits until the Keto read API answers.
func ensureKetoReady(readURL string) error {
	if readURL == "" {
		return errors.New("keto url not set")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(strings.TrimRight(readURL, "/") + "/health/ready")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("keto not ready")
}
