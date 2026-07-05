package middleware

import (
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	Secret      string
	PrevSecret  string
	ContextKey  string
	TokenLookup string
	Algorithm   string
	Issuer      string
	Audience    string
}

func DefaultJWTConfig() JWTConfig {
	return JWTConfig{
		Secret:      "",
		PrevSecret:  "",
		TokenLookup: "header:Authorization",
		Algorithm:   "HS256",
	}
}

func JWT(cfg JWTConfig) fiber.Handler {
	if cfg.ContextKey == "" {
		cfg.ContextKey = "claims"
	}
	if cfg.TokenLookup == "" {
		cfg.TokenLookup = "header:Authorization"
	}
	if cfg.Algorithm == "" {
		cfg.Algorithm = "HS256"
	}

	currentParser := newParser(cfg)
	var prevParser *jwtParser
	if cfg.PrevSecret != "" {
		prevCfg := cfg
		prevCfg.Secret = cfg.PrevSecret
		prevParser = newParser(prevCfg)
	}

	return func(c fiber.Ctx) error {
		token, rawToken := extractToken(c, cfg.TokenLookup)
		if token == "" {
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

		if err := validateClaims(claims, cfg); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": err.Error(),
			})
		}

		c.Locals(cfg.ContextKey, claims)
		injectAuth(c, buildAuthContext(claims, rawToken))
		return c.Next()
	}
}

func validateClaims(claims jwt.MapClaims, cfg JWTConfig) error {
	if cfg.Issuer != "" {
		iss, err := claims.GetIssuer()
		if err != nil || iss != cfg.Issuer {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid issuer")
		}
	}
	if cfg.Audience != "" {
		aud, err := claims.GetAudience()
		if err != nil || !containsString(aud, cfg.Audience) {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid audience")
		}
	}
	exp, err := claims.GetExpirationTime()
	if err == nil && exp != nil && exp.Before(time.Now()) {
		return fiber.NewError(fiber.StatusUnauthorized, "token expired")
	}
	return nil
}

func containsString(slice []string, target string) bool {
	return slices.Contains(slice, target)
}

type jwtParser struct {
	secret    []byte
	algorithm string
}

func newParser(cfg JWTConfig) *jwtParser {
	return &jwtParser{secret: []byte(cfg.Secret), algorithm: cfg.Algorithm}
}

func (p *jwtParser) parse(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != p.algorithm {
			return nil, jwt.ErrSignatureInvalid
		}
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

func extractToken(c fiber.Ctx, lookup string) (token, raw string) {
	parts := strings.SplitN(lookup, ":", 2)
	if len(parts) != 2 {
		return "", ""
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
		return "", ""
	}

	const prefix = "Bearer "
	if after, ok := strings.CutPrefix(value, prefix); ok {
		return after, value
	}
	return value, value
}
