package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTOTPSecret(t *testing.T) {
	t.Parallel()
	secret, err := GenerateTOTPSecret()
	require.NoError(t, err)
	require.NotEmpty(t, secret)
	// Base32 encoded, no padding
	assert.Less(t, len(secret), 40)
	assert.False(t, strings.HasSuffix(secret, "="))
}

func TestGenerateTOTPSecret_Unique(t *testing.T) {
	t.Parallel()
	s1, _ := GenerateTOTPSecret()
	s2, _ := GenerateTOTPSecret()
	assert.NotEqual(t, s1, s2)
}

func TestPadBase32(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"A", 0},
		{"AB", 8},
		{"ABCD", 8},
		{"ABCDE", 8},
		{"ABCDEFG", 8},
		{"ABCDEFGH", 8},
	}
	for _, tt := range tests {
		padded := padBase32(tt.input)
		if tt.want > 0 {
			assert.Len(t, padded, tt.want)
		} else {
			assert.Len(t, padded, len(tt.input))
		}
	}
}

func TestGenerateTOTPURI(t *testing.T) {
	t.Parallel()
	uri := GenerateTOTPURI("SECRET", "MyApp", "user@example.com")
	assert.Contains(t, uri, "otpauth://totp/")
	assert.Contains(t, uri, "secret=SECRET")
	assert.Contains(t, uri, "issuer=MyApp")
	assert.Contains(t, uri, "algorithm=SHA256")
}

func TestHotp_Deterministic(t *testing.T) {
	t.Parallel()
	// Same key + same counter = same code
	key := []byte("test-key-12345")
	code1 := hotp(key, 0)
	code2 := hotp(key, 0)
	assert.Equal(t, code1, code2)
	assert.Len(t, code1, 6)
}

func TestHotp_DifferentCounter(t *testing.T) {
	t.Parallel()
	key := []byte("test-key")
	code0 := hotp(key, 0)
	code1 := hotp(key, 1)
	assert.NotEqual(t, code0, code1)
}

func TestHotp_NegativeCounter(t *testing.T) {
	t.Parallel()
	key := []byte("test-key")
	code := hotp(key, -1)
	assert.Len(t, code, 6)
	assert.NotEmpty(t, code)
}

func TestGenerateTOTPCode(t *testing.T) {
	t.Parallel()
	secret, err := GenerateTOTPSecret()
	require.NoError(t, err)
	code := GenerateTOTPCode(secret)
	assert.Len(t, code, 6)
}

func TestGenerateTOTPCode_InvalidSecret(t *testing.T) {
	t.Parallel()
	code := GenerateTOTPCode("!!!invalid!!!")
	assert.Empty(t, code)
}

func TestValidateTOTP_InvalidSecret(t *testing.T) {
	t.Parallel()
	assert.False(t, ValidateTOTP("!!!", "000000"))
}

func TestValidateTOTP_EmptySecret(t *testing.T) {
	t.Parallel()
	assert.False(t, ValidateTOTP("", "000000"))
}
