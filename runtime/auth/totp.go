package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// GenerateTOTPSecret generates a random base32-encoded TOTP secret (160 bits).
func GenerateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(secret), "="), nil
}

// ValidateTOTP checks a 6-digit code against a TOTP secret.
// Checks current, previous, and next time step to allow for clock drift.
func ValidateTOTP(secret, code string) bool {
	key, err := base32.StdEncoding.DecodeString(padBase32(secret))
	if err != nil || len(key) == 0 {
		return false
	}
	now := time.Now().Unix()
	for offset := int64(-1); offset <= 1; offset++ {
		if hotp(key, now/30+offset) == code {
			return true
		}
	}
	return false
}

// GenerateTOTPURI builds an otpauth:// URI for use with authenticator apps.
func GenerateTOTPURI(secret, issuer, accountName string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA256&digits=6&period=30",
		issuer, accountName, secret, issuer)
}

func padBase32(s string) string {
	switch len(s) % 8 {
	case 2:
		return s + "======"
	case 4:
		return s + "===="
	case 5:
		return s + "==="
	case 7:
		return s + "="
	}
	return s
}

// GenerateTOTPCode generates the current TOTP code for a given secret.
func GenerateTOTPCode(secret string) string {
	key, err := base32.StdEncoding.DecodeString(padBase32(secret))
	if err != nil || len(key) == 0 {
		return ""
	}
	return hotp(key, time.Now().Unix()/30)
}

// hotp implements RFC 4226 HOTP with SHA-256, 6 digits.
func hotp(key []byte, counter int64) string {
	buf := make([]byte, 8)
	if counter < 0 {
		counter = 0
	}
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha256.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 0xf
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code := truncated % 1000000
	return fmt.Sprintf("%06d", code)
}
