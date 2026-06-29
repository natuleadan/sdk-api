package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	Secret      string
	PrevSecret  string
	ContextKey  string
	TokenLookup string
}

func DefaultJWTConfig() JWTConfig {
	return JWTConfig{
		Secret:      "",
		PrevSecret:  "",
		ContextKey:  "claims",
		TokenLookup: "header:Authorization",
	}
}

func JWT(cfg JWTConfig) fiber.Handler {
	if cfg.ContextKey == "" {
		cfg.ContextKey = "claims"
	}
	if cfg.TokenLookup == "" {
		cfg.TokenLookup = "header:Authorization"
	}

	currentParser := newParser(cfg.Secret)
	var prevParser *jwtParser
	if cfg.PrevSecret != "" {
		prevParser = newParser(cfg.PrevSecret)
	}

	return func(c *fiber.Ctx) error {
		token, err := extractToken(c, cfg.TokenLookup)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "missing or malformed token",
			})
		}

		claims, err := currentParser.parse(token)
		if err != nil && prevParser != nil {
			claims, err = prevParser.parse(token)
		}
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "invalid or expired token",
			})
		}

		c.Locals(cfg.ContextKey, claims)
		return c.Next()
	}
}

type jwtParser struct {
	secret []byte
}

func newParser(secret string) *jwtParser {
	return &jwtParser{secret: []byte(secret)}
}

func (p *jwtParser) parse(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return p.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}

func extractToken(c *fiber.Ctx, lookup string) (string, error) {
	parts := strings.SplitN(lookup, ":", 2)
	if len(parts) != 2 {
		return "", fiber.ErrBadRequest
	}

	source := parts[0]
	key := parts[1]

	var value string
	switch source {
	case "header":
		value = c.Get(key)
	case "cookie":
		value = c.Cookies(key)
	case "query":
		value = c.Query(key)
	default:
		return "", fiber.ErrBadRequest
	}

	const prefix = "Bearer "
	if strings.HasPrefix(value, prefix) {
		return strings.TrimPrefix(value, prefix), nil
	}
	return value, nil
}
