package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type CSRFConfig struct {
	Enabled      bool     `json:"enabled" config:",optional"`
	CookieName   string   `json:"cookie_name" config:",optional"`
	HeaderName   string   `json:"header_name" config:",optional"`
	SameSite     string   `json:"same_site" config:",optional"`
	Secure       bool     `json:"secure" config:",optional"`
	ExcludePaths []string `json:"exclude_paths" config:",optional"`
	JSONCheck    bool     `json:"json_check" config:",optional"`
}

func CSRF(cfg CSRFConfig) fiber.Handler {
	cookieName := cfg.CookieName
	if cookieName == "" {
		cookieName = "csrf_token"
	}
	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "X-CSRF-Token"
	}
	sameSite := parseSameSite(cfg.SameSite)

	return func(c *fiber.Ctx) error {
		if isExcludedPath(c.Path(), cfg.ExcludePaths) {
			return c.Next()
		}
		if c.Locals("csrf_skip") == true {
			return c.Next()
		}

		if c.Method() == "GET" || c.Method() == "HEAD" || c.Method() == "OPTIONS" {
			token, err := generateCSRFToken()
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "csrf token generation failed"})
			}
			c.Cookie(&fiber.Cookie{
				Name:     cookieName,
				Value:    token,
				Path:     "/",
				Secure:   cfg.Secure,
				SameSite: sameSite,
				HTTPOnly: false,
			})
			c.Locals(cookieName, token)
			return c.Next()
		}

		// JSON CSRF check: if body is JSON, browser Same-Origin Policy protects it.
		// If body is NOT JSON (e.g. form-urlencoded), CSRF is still required.
		if cfg.JSONCheck && isJSONContentType(c) {
			return c.Next()
		}

		headerToken := c.Get(headerName)
		cookieToken := c.Cookies(cookieName)
		if headerToken == "" || cookieToken == "" || headerToken != cookieToken {
			return c.Status(403).JSON(fiber.Map{
				"error": "csrf token mismatch",
			})
		}
		return c.Next()
	}
}

func isJSONContentType(c *fiber.Ctx) bool {
	ct := c.Get("Content-Type")
	return strings.HasPrefix(ct, "application/json")
}

func isExcludedPath(path string, excludePaths []string) bool {
	for _, ep := range excludePaths {
		if ep == "" {
			continue
		}
		if strings.HasSuffix(ep, "/*") {
			prefix := strings.TrimSuffix(ep, "/*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		} else if ep == path {
			return true
		}
	}
	return false
}

func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func parseSameSite(s string) string {
	switch strings.ToLower(s) {
	case "strict":
		return "Strict"
	case "lax":
		return "Lax"
	case "none":
		return "None"
	default:
		return "Lax"
	}
}
