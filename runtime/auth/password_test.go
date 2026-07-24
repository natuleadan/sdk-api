package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("my-secure-password")
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	assert.Greater(t, len(hash), 20)
}

func TestVerifyPassword_Correct(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("correct-password")
	require.NoError(t, err)
	assert.True(t, VerifyPassword(hash, "correct-password"))
}

func TestVerifyPassword_Wrong(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("real-password")
	require.NoError(t, err)
	assert.False(t, VerifyPassword(hash, "wrong-password"))
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	t.Parallel()
	assert.False(t, VerifyPassword("invalid-hash", "password"))
}

func TestVerifyPassword_EmptyPassword(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("something")
	require.NoError(t, err)
	assert.False(t, VerifyPassword(hash, ""))
}

func TestGenerateToken(t *testing.T) {
	t.Parallel()
	token, err := GenerateToken()
	require.NoError(t, err)
	assert.Len(t, token, 64) // 32 bytes = 64 hex chars

	token2, err := GenerateToken()
	require.NoError(t, err)
	assert.NotEqual(t, token, token2)
}

func TestTokenHash(t *testing.T) {
	t.Parallel()
	h := TokenHash("my-api-key")
	assert.Len(t, h, 64) // SHA-256 = 64 hex chars

	h2 := TokenHash("my-api-key")
	assert.Equal(t, h, h2)

	h3 := TokenHash("other-key")
	assert.NotEqual(t, h, h3)
}

func TestCheckPasswordStrength_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, CheckPasswordStrength("Abcdef1!"))
	assert.NoError(t, CheckPasswordStrength("Password1"))
}

func TestCheckPasswordStrength_TooShort(t *testing.T) {
	t.Parallel()
	err := CheckPasswordStrength("Ab1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8")
}

func TestCheckPasswordStrength_NoUpper(t *testing.T) {
	t.Parallel()
	err := CheckPasswordStrength("abcdefgh1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uppercase")
}

func TestCheckPasswordStrength_NoLower(t *testing.T) {
	t.Parallel()
	err := CheckPasswordStrength("ABCDEFG1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lowercase")
}

func TestCheckPasswordStrength_NoDigit(t *testing.T) {
	t.Parallel()
	err := CheckPasswordStrength("Abcdefgh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digit")
}
