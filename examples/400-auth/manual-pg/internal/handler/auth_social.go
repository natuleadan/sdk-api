package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"auth-roles/internal/svc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/runtime/auth"
	"github.com/natuleadan/sdk-api/server/middleware"
	"golang.org/x/oauth2"
)

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func buildStateJWT(secret, provider string) (string, error) {
	claims := map[string]any{
		"sub":      randomHex(16),
		"provider": provider,
		"purpose":  "oauth_state",
		"exp":      time.Now().Add(10 * time.Minute).Unix(),
		"iat":      time.Now().Unix(),
	}
	return middleware.SignToken(secret, "HS256", claims)
}

func verifyStateJWT(secret, tokenStr, expectedProvider string) bool {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false
	}
	if claims["purpose"] != "oauth_state" {
		return false
	}
	if claims["provider"] != expectedProvider {
		return false
	}
	return true
}

func handleSocialLogin(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		provider := c.Params("provider")
		p := getProvider(provider)
		if p == nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "unsupported provider"})
		}

		stateToken, err := buildStateJWT(svcCtx.JWTSecret, provider)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "state generation failed"})
		}

		if !p.hasCredentials() {
			mockCode := randomHex(16)
			log.Printf("[SOCIAL MOCK] %s login: state=%s code=%s", provider, stateToken, mockCode)
			return c.JSON(runtime.Map{
				"provider":  provider,
				"state":     stateToken,
				"mock_code": mockCode,
			})
		}

		redirectURL := p.OAuth2Config.AuthCodeURL(stateToken, oauth2.AccessTypeOnline)
		return c.Redirect(redirectURL)
	}
}

func handleSocialCallback(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		provider := c.Params("provider")
		p := getProvider(provider)
		if p == nil {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "unsupported provider"})
		}

		stateToken := c.Query("state")
		code := c.Query("code")

		var providerID, email, name string

		if !p.hasCredentials() && (stateToken == "" || code == "") {
			var body struct {
				State      string `json:"state"`
				ProviderID string `json:"provider_id"`
				Email      string `json:"email"`
				Name       string `json:"name"`
			}
			if err := c.Bind(&body); err != nil || body.State == "" {
				return c.Status(400).JSON(runtime.Map{"code": 400, "message": "state and provider_id required in mock mode"})
			}
			stateToken = body.State
			providerID = body.ProviderID
			email = body.Email
			name = body.Name
		}

		if !verifyStateJWT(svcCtx.JWTSecret, stateToken, provider) {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "invalid or expired state token"})
		}

		if !p.hasCredentials() && providerID == "" {
			providerID = code
			email = code + "@" + provider + ".mock"
			n := len(code)
			if n > 8 {
				n = 8
			}
			name = provider + "-user-" + code[:n]
		}

		if p.hasCredentials() {
			if stateToken == "" || code == "" {
				return c.Status(400).JSON(runtime.Map{"code": 400, "message": "state and code required"})
			}

			token, err := p.exchange(c.Context(), code)
			if err != nil {
				return c.Status(401).JSON(runtime.Map{"code": 401, "message": fmt.Sprintf("token exchange failed: %v", err)})
			}

			userInfo, err := p.fetchUserInfo(c.Context(), token)
			if err != nil {
				return c.Status(401).JSON(runtime.Map{"code": 401, "message": fmt.Sprintf("userinfo failed: %v", err)})
			}

			providerID, _ = userInfo["sub"].(string)
			if id, ok := userInfo["id"].(string); ok && providerID == "" {
				providerID = id
			}
			email, _ = userInfo["email"].(string)
			name, _ = userInfo["name"].(string)
			if name == "" {
				name, _ = userInfo["login"].(string)
			}
		}

		if providerID == "" {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "could not identify user from provider"})
		}

		userID, err := findOrCreateLinkedUser(c, provider, providerID, email, name)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}

		var role string
		pool := c.PoolPG("primary")
		pool.QueryRow(c.Context(), `SELECT role FROM users WHERE id = $1`, userID).Scan(&role)
		if role == "" {
			role = "viewer"
		}

		sessionToken, err := middleware.SignToken(svcCtx.JWTSecret, "HS256",
			middleware.DefaultClaims(userID, "", []string{role}, nil, svcCtx.AuthExpiry))
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": "session creation failed"})
		}

		c.SetCookie(runtime.NewCookie("token", sessionToken, svcCtx.AuthExpiry))
		log.Printf("[SOCIAL] user %s logged in via %s (provider_id=%s)", email, provider, providerID)
		return c.JSON(runtime.Map{"token": sessionToken, "role": role, "user_id": userID, "provider": provider})
	}
}

func findOrCreateLinkedUser(c *runtime.RestCtx, provider, providerID, email, name string) (string, error) {
	pool := c.PoolPG("primary")
	ctx := c.Context()

	var userID string
	err := pool.QueryRow(ctx,
		`SELECT user_id FROM linked_accounts WHERE provider = $1 AND provider_id = $2`,
		provider, providerID).Scan(&userID)
	if err == nil {
		return userID, nil
	}

	if email != "" {
		err = pool.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, email).Scan(&userID)
		if err == nil {
			_, _ = pool.Exec(ctx,
				`INSERT INTO linked_accounts (user_id, provider, provider_id, email) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
				userID, provider, providerID, email)
			return userID, nil
		}
	}

	username := email
	if username == "" {
		n := len(providerID)
		if n > 8 {
			n = 8
		}
		username = provider + "-" + providerID[:n]
	}
	userID = "user-" + username
	hash, _ := hashPassword(randomHex(16))
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, username, password_hash, role) VALUES ($1, $2, $3, 'viewer') ON CONFLICT (username) DO NOTHING`,
		userID, username, hash)
	if err != nil {
		return "", fmt.Errorf("user creation failed: %w", err)
	}

	_, _ = pool.Exec(ctx,
		`INSERT INTO linked_accounts (user_id, provider, provider_id, email) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		userID, provider, providerID, email)
	return userID, nil
}

func handleLinkedAccounts(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		pool := c.PoolPG("primary")
		rows, err := pool.Query(c.Context(),
			`SELECT id, provider, provider_id, email FROM linked_accounts WHERE user_id = $1 ORDER BY provider`, a.UserID)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		defer rows.Close()
		var accounts []map[string]any
		for rows.Next() {
			var id, provider, providerID, email string
			rows.Scan(&id, &provider, &providerID, &email)
			accounts = append(accounts, runtime.Map{"id": id, "provider": provider, "provider_id": providerID, "email": email})
		}
		if accounts == nil {
			accounts = []map[string]any{}
		}
		return c.JSON(runtime.Map{"data": accounts})
	}
}

func handleLinkAccount(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		var body struct {
			Provider   string `json:"provider"`
			ProviderID string `json:"provider_id"`
			Email      string `json:"email"`
		}
		if err := c.Bind(&body); err != nil || body.Provider == "" || body.ProviderID == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "provider and provider_id required"})
		}
		pool := c.PoolPG("primary")
		_, err := pool.Exec(c.Context(),
			`INSERT INTO linked_accounts (user_id, provider, provider_id, email) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			a.UserID, body.Provider, body.ProviderID, body.Email)
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"code": 500, "message": err.Error()})
		}
		return c.JSON(runtime.Map{"status": "linked"})
	}
}

func handleUnlinkAccount(svcCtx *svc.ServiceContext) func(c *runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := getAuth(c)
		if a == nil {
			return c.Status(401).JSON(runtime.Map{"code": 401, "message": "unauthorized"})
		}
		accountID := c.Params("id")
		if accountID == "" {
			return c.Status(400).JSON(runtime.Map{"code": 400, "message": "account id required"})
		}
		pool := c.PoolPG("primary")
		var ownerID string
		err := pool.QueryRow(c.Context(),
			`SELECT user_id FROM linked_accounts WHERE id = $1`, accountID).Scan(&ownerID)
		if err != nil {
			return c.Status(404).JSON(runtime.Map{"code": 404, "message": "account not found"})
		}
		if ownerID != a.UserID {
			return c.Status(403).JSON(runtime.Map{"code": 403, "message": "cannot unlink another user's account"})
		}
		_, _ = pool.Exec(c.Context(), `DELETE FROM linked_accounts WHERE id = $1`, accountID)
		return c.JSON(runtime.Map{"status": "unlinked"})
	}
}

func hashPassword(password string) (string, error) {
	return auth.HashPassword(password)
}
