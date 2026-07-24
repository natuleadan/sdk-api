package runtime

import (
	"crypto/rand"
	"math/big"
	"time"
)

// GenerateShortCode generates a random alphanumeric string of given length.
// Uses crypto/rand for security-sensitive contexts (URL shorteners, tokens).
func GenerateShortCode(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, n)
	for i := range code {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		code[i] = charset[idx.Int64()]
	}
	return string(code)
}

// PresignTTL extracts the presign TTL duration from a StorageBackend.
// Returns 0 if the backend does not support presigned URLs.
func PresignTTL(store any) time.Duration {
	if p, ok := store.(interface{ PresignTTL() time.Duration }); ok {
		return p.PresignTTL()
	}
	return 0
}
