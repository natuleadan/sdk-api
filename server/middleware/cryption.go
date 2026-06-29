package middleware

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func Cryption(key []byte) fiber.Handler {
	return func(c *fiber.Ctx) error {
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
	if err != nil { return nil, err }
	if len(data) < aes.BlockSize { return nil, fmt.Errorf("too short") }
	iv := data[:aes.BlockSize]
	data = data[aes.BlockSize:]
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(data, data)
	return data, nil
}

func AESEncrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil { return nil, err }
	ciphertext := make([]byte, aes.BlockSize+len(data))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], data)
	var buf bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	encoder.Write(ciphertext)
	encoder.Close()
	return buf.Bytes(), nil
}
