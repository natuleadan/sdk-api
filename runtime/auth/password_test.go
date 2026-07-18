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
