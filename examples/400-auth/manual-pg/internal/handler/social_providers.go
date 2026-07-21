package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
)

type oauth2Provider struct {
	Name         string
	OAuth2Config *oauth2.Config
	UserInfoURL  string
}

func (p *oauth2Provider) clientID() string {
	if p.OAuth2Config != nil {
		return p.OAuth2Config.ClientID
	}
	return ""
}

func (p *oauth2Provider) clientSecret() string {
	if p.OAuth2Config != nil {
		return p.OAuth2Config.ClientSecret
	}
	return ""
}

func (p *oauth2Provider) hasCredentials() bool {
	return p.clientID() != "" && p.clientSecret() != ""
}

func (p *oauth2Provider) authRedirectURI() string {
	return "http://localhost:23400/api/auth/" + p.Name + "/callback"
}

func (p *oauth2Provider) exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	if p.OAuth2Config == nil {
		return nil, fmt.Errorf("oauth2 config not initialized")
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	return p.OAuth2Config.Exchange(ctx, code)
}

func (p *oauth2Provider) fetchUserInfo(ctx context.Context, token *oauth2.Token) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read userinfo: %w", err)
	}

	var userInfo map[string]any
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("parse userinfo: %w", err)
	}
	return userInfo, nil
}

func getProvider(name string) *oauth2Provider {
	switch name {
	case "google":
		return &oauth2Provider{
			Name: "google",
			OAuth2Config: &oauth2.Config{
				ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
				ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
				Endpoint: oauth2.Endpoint{
					AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
					TokenURL: "https://oauth2.googleapis.com/token",
				},
				Scopes:      []string{"openid", "email", "profile"},
				RedirectURL: "http://localhost:23400/api/auth/google/callback",
			},
			UserInfoURL: "https://www.googleapis.com/oauth2/v2/userinfo",
		}
	case "github":
		return &oauth2Provider{
			Name: "github",
			OAuth2Config: &oauth2.Config{
				ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
				ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
				Endpoint: oauth2.Endpoint{
					AuthURL:  "https://github.com/login/oauth/authorize",
					TokenURL: "https://github.com/login/oauth/access_token",
				},
				Scopes:      []string{"read:user", "user:email"},
				RedirectURL: "http://localhost:23400/api/auth/github/callback",
			},
			UserInfoURL: "https://api.github.com/user",
		}
	}
	return nil
}
