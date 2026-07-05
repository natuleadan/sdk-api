package middleware

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func ContentSecurity(key *rsa.PublicKey, strict bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		sig := c.Get("X-Content-Security")
		if sig == "" {
			if strict {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"code": 403, "message": "missing content security signature",
				})
			}
			return c.Next()
		}
		sigBytes, err := base64.StdEncoding.DecodeString(sig)
		if err != nil {
			logx.Errorf("content security decode: %v", err)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code": 403, "message": "invalid signature encoding",
			})
		}
		body := c.Body()
		h := sha256.Sum256(body)
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, h[:], sigBytes); err != nil {
			logx.Errorf("content security verify failed: %v", err)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code": 403, "message": "invalid signature",
			})
		}
		return c.Next()
	}
}

func SignBody(key *rsa.PrivateKey, body []byte) (string, error) {
	h := sha256.Sum256(body)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func ParsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	key, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not RSA")
	}
	return key, nil
}
