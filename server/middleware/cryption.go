package middleware

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func Cryption(key []byte) fiber.Handler {
	return func(c fiber.Ctx) error {
		encrypted := c.Body()
		if len(encrypted) == 0 {
			return c.Next()
		}
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(encrypted)))
		n, err := base64.StdEncoding.Decode(decoded, encrypted)
		if err != nil {
			logx.Errorf("cryption decode: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"code": 400, "message": "invalid encryption format",
			})
		}
		decrypted, err := aesDecrypt(decoded[:n], key)
		if err != nil {
			logx.Errorf("cryption decrypt: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"code": 400, "message": "decryption failed",
			})
		}
		c.Request().SetBody(decrypted)
		return c.Next()
	}
}

func aesDecrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := aead.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return aead.Open(nil, nonce, ciphertext, nil)
}

func AESEncrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nonce, nonce, data, nil)
	var buf bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	if _, err := encoder.Write(ciphertext); err != nil {
		logx.Errorf("cryption: encoder write error: %v", err)
	}
	if err := encoder.Close(); err != nil {
		logx.Errorf("cryption: encoder close error: %v", err)
	}
	return buf.Bytes(), nil
}
