package middleware

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidKey = errors.New("invalid private key")

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
	rsaPub    *rsa.PublicKey
	ecdsaPub  *ecdsa.PublicKey
	algorithm string
}

func newParser(cfg JWTConfig) *jwtParser {
	p := &jwtParser{algorithm: cfg.Algorithm}
	switch {
	case strings.HasPrefix(cfg.Algorithm, "HS"):
		p.secret = []byte(cfg.Secret)
	case strings.HasPrefix(cfg.Algorithm, "RS"):
		p.rsaPub = parseRSAPublicKey([]byte(cfg.Secret))
	case strings.HasPrefix(cfg.Algorithm, "ES"):
		p.ecdsaPub = parseECDSAPublicKey([]byte(cfg.Secret))
	}
	return p
}

func parseRSAPublicKey(pemBytes []byte) *rsa.PublicKey {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil
	}
	return pub
}

func parseECDSAPublicKey(pemBytes []byte) *ecdsa.PublicKey {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil
	}
	pub, ok := key.(*ecdsa.PublicKey)
	if !ok {
		return nil
	}
	return pub
}

func parseRSAPrivateKey(pemBytes []byte) any {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	if err != nil {
		return nil
	}
	return key
}

func parseECDSAPrivateKey(pemBytes []byte) any {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParseECPrivateKey(block.Bytes)
	}
	if err != nil {
		return nil
	}
	return key
}

func (p *jwtParser) parse(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != p.algorithm {
			return nil, jwt.ErrSignatureInvalid
		}
		switch {
		case strings.HasPrefix(p.algorithm, "HS"):
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return p.secret, nil
		case strings.HasPrefix(p.algorithm, "RS"):
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			if p.rsaPub == nil {
				return nil, jwt.ErrSignatureInvalid
			}
			return p.rsaPub, nil
		case strings.HasPrefix(p.algorithm, "ES"):
			if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			if p.ecdsaPub == nil {
				return nil, jwt.ErrSignatureInvalid
			}
			return p.ecdsaPub, nil
		}
		return nil, jwt.ErrSignatureInvalid
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

// SignToken creates and signs a JWT using the given secret and algorithm.
// Supported algorithms: HS256, HS384, HS512, RS256, RS384, RS512, ES256, ES384, ES512.
// For RS* and ES*, secret must be a PEM-encoded private key.
func SignToken(secret string, algorithm string, claims map[string]any) (string, error) {
	var method jwt.SigningMethod
	switch algorithm {
	case "HS384":
		method = jwt.SigningMethodHS384
	case "HS512":
		method = jwt.SigningMethodHS512
	case "RS256":
		method = jwt.SigningMethodRS256
	case "RS384":
		method = jwt.SigningMethodRS384
	case "RS512":
		method = jwt.SigningMethodRS512
	case "ES256":
		method = jwt.SigningMethodES256
	case "ES384":
		method = jwt.SigningMethodES384
	case "ES512":
		method = jwt.SigningMethodES512
	default:
		method = jwt.SigningMethodHS256
	}
	tok := jwt.NewWithClaims(method, jwt.MapClaims(claims))
	var key any
	switch {
	case strings.HasPrefix(algorithm, "HS"):
		key = []byte(secret)
	case strings.HasPrefix(algorithm, "RS"):
		key = parseRSAPrivateKey([]byte(secret))
		if key == nil {
			return "", ErrInvalidKey
		}
	case strings.HasPrefix(algorithm, "ES"):
		key = parseECDSAPrivateKey([]byte(secret))
		if key == nil {
			return "", ErrInvalidKey
		}
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", err
	}
	return signed, nil
}

// DefaultClaims builds standard JWT claims for a user session.
func DefaultClaims(sub, orgID string, roles, permissions []string, ttlSeconds int) map[string]any {
	now := time.Now().Unix()
	claims := map[string]any{
		"sub": sub,
		"iat": now,
		"exp": now + int64(ttlSeconds),
	}
	if orgID != "" {
		claims["org_id"] = orgID
	}
	if len(roles) > 0 {
		claims["roles"] = roles
	}
	if len(permissions) > 0 {
		claims["permissions"] = permissions
	}
	return claims
}
