package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignAndParseToken_RoundTrip(t *testing.T) {
	secret := "test-secret-hs256"
	claims := map[string]any{
		"sub":      "user-123",
		"role":     "admin",
		"purpose":  "test",
		"exp":      time.Now().Add(time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	tok, err := SignToken(secret, "HS256", claims)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	parsed, err := ParseToken(tok, secret, "HS256")
	require.NoError(t, err)
	assert.Equal(t, "user-123", parsed["sub"])
	assert.Equal(t, "admin", parsed["role"])
	assert.Equal(t, "test", parsed["purpose"])
}

func TestParseToken_WrongSecret(t *testing.T) {
	tok, err := SignToken("secret-a", "HS256", map[string]any{"sub": "x"})
	require.NoError(t, err)

	_, err = ParseToken(tok, "secret-b", "HS256")
	require.Error(t, err)
}

func TestParseToken_DefaultAlgorithm(t *testing.T) {
	tok, err := SignToken("secret", "", map[string]any{"sub": "x"})
	require.NoError(t, err)

	_, err = ParseToken(tok, "secret", "")
	require.NoError(t, err)
}

func TestParseToken_Expired(t *testing.T) {
	tok, err := SignToken("secret", "HS256", map[string]any{
		"sub": "x",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	require.NoError(t, err)

	_, err = ParseToken(tok, "secret", "HS256")
	require.Error(t, err)
}

func TestParseTokenUnverified(t *testing.T) {
	tok, err := SignToken("secret", "HS256", map[string]any{
		"sub":  "user-1",
		"role": "viewer",
	})
	require.NoError(t, err)

	claims, err := ParseTokenUnverified(tok)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims["sub"])
	assert.Equal(t, "viewer", claims["role"])
}

func TestParseTokenUnverified_Invalid(t *testing.T) {
	_, err := ParseTokenUnverified("not-a-jwt")
	require.Error(t, err)
}

func TestParseTokenUnverified_WrongSignatureStillParses(t *testing.T) {
	tok, err := SignToken("correct-secret", "HS256", map[string]any{"sub": "x"})
	require.NoError(t, err)

	// Unverified parse must succeed even though signature would not validate
	claims, err := ParseTokenUnverified(tok)
	require.NoError(t, err)
	assert.Equal(t, "x", claims["sub"])
}
