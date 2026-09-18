package main

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

// zitadelBootstrap holds the identifiers the tests need: the project audience
// and the introspection client credentials.
type zitadelBootstrap struct {
	Issuer       string `json:"issuer"`
	ProjectID    string `json:"project_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type machineKey struct {
	KeyID  string `json:"keyId"`
	Key    string `json:"key"`
	UserID string `json:"userId"`
}

// bootstrapZitadel creates (idempotently) a project and an API application with
// a client secret, using the machine key. The application is what the service
// uses to introspect opaque access tokens, so the example needs no manual
// Zitadel setup.
func bootstrapZitadel(issuer, machineKeyPath string) (*zitadelBootstrap, error) {
	if issuer == "" {
		issuer = "http://localhost:18082"
	}
	issuer = strings.TrimRight(issuer, "/")

	raw, err := os.ReadFile(machineKeyPath)
	if err != nil {
		return nil, fmt.Errorf("machine key: %w", err)
	}
	var mk machineKey
	if err := json.Unmarshal(raw, &mk); err != nil {
		return nil, fmt.Errorf("machine key decode: %w", err)
	}
	priv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(mk.Key))
	if err != nil {
		return nil, fmt.Errorf("machine key pem: %w", err)
	}

	api, err := machineAccessToken(issuer, mk, priv, "openid urn:zitadel:iam:org:project:id:zitadel:aud")
	if err != nil {
		return nil, err
	}

	// Recreate the example project for a fresh client secret every boot.
	st, body, err := zitadelCall(issuer, http.MethodPost, "/management/v1/projects/_search", api, map[string]any{"queries": []any{}})
	if err != nil {
		return nil, err
	}
	var list struct {
		Result []struct{ ID, Name string }
	}
	_ = json.Unmarshal([]byte(body), &list)
	for _, p := range list.Result {
		if p.Name == "nla-example" {
			_, _, _ = zitadelCall(issuer, http.MethodDelete, "/management/v1/projects/"+p.ID, api, nil)
		}
	}

	st, body, err = zitadelCall(issuer, http.MethodPost, "/management/v1/projects", api, map[string]any{"name": "nla-example"})
	if err != nil {
		return nil, err
	}
	var project struct{ ID string }
	if err := json.Unmarshal([]byte(body), &project); err != nil || project.ID == "" {
		return nil, fmt.Errorf("create project (%d): %s", st, body)
	}

	st, body, err = zitadelCall(issuer, http.MethodPost, "/management/v1/projects/"+project.ID+"/apps/api", api,
		map[string]any{"name": "nla-example-api", "authMethodType": "API_AUTH_METHOD_TYPE_BASIC"})
	if err != nil {
		return nil, err
	}
	var app struct{ ClientID, ClientSecret string }
	if err := json.Unmarshal([]byte(body), &app); err != nil || app.ClientID == "" {
		return nil, fmt.Errorf("create app (%d): %s", st, body)
	}

	return &zitadelBootstrap{
		Issuer:       issuer,
		ProjectID:    project.ID,
		ClientID:     app.ClientID,
		ClientSecret: app.ClientSecret,
	}, nil
}

func machineAccessToken(issuer string, mk machineKey, priv *rsa.PrivateKey, scope string) (string, error) {
	now := time.Now()
	assertion := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": mk.UserID, "sub": mk.UserID, "aud": issuer,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	})
	assertion.Header["kid"] = mk.KeyID
	signed, err := assertion.SignedString(priv)
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", signed)
	form.Set("scope", scope)
	resp, err := http.PostForm(issuer+"/oauth/v2/token", form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("token (%d): %s", resp.StatusCode, string(body))
	}
	return out.AccessToken, nil
}

func zitadelCall(issuer, method, path, token string, payload any) (int, string, error) {
	var r io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, issuer+path, r)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

// writeBootstrapInfo persists the identifiers for the test process.
func writeBootstrapInfo(path string, info *zitadelBootstrap) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(path, b, 0o600)
}
