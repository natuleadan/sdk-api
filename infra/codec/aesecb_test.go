package codec

import (
	"crypto/aes"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAesEcb(t *testing.T) {
	var (
		key     = []byte("q4t7w!z%C*F-JaNdRgUjXn2r5u8x/A?D")
		val     = []byte("helloworld")
		valLong = []byte("helloworldlong..")
		badKey1 = []byte("aaaaaaaaa")
		// more than 32 chars
		badKey2 = []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	)
	_, err := EcbEncrypt(badKey1, val)
	require.Error(t, err)
	_, err = EcbEncrypt(badKey2, val)
	require.Error(t, err)
	dst, err := EcbEncrypt(key, val)
	require.NoError(t, err)
	_, err = EcbDecrypt(badKey1, dst)
	require.Error(t, err)
	_, err = EcbDecrypt(badKey2, dst)
	require.Error(t, err)
	_, err = EcbDecrypt(key, val)
	// not a multiple of block size
	require.Error(t, err)
	src, err := EcbDecrypt(key, dst)
	require.NoError(t, err)
	assert.Equal(t, val, src)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	encrypter := NewECBEncrypter(block)
	assert.Equal(t, 16, encrypter.BlockSize())
	decrypter := NewECBDecrypter(block)
	assert.Equal(t, 16, decrypter.BlockSize())

	dst = make([]byte, 8)
	assert.Panics(t, func() {
		encrypter.CryptBlocks(dst, val)
	})

	dst = make([]byte, 8)
	assert.Panics(t, func() {
		encrypter.CryptBlocks(dst, valLong)
	})

	dst = make([]byte, 8)
	assert.Panics(t, func() {
		decrypter.CryptBlocks(dst, val)
	})

	dst = make([]byte, 8)
	assert.Panics(t, func() {
		decrypter.CryptBlocks(dst, valLong)
	})

	_, err = EcbEncryptBase64("cTR0N3dDKkYtSmFOZFJnVWpYbjJyNXU4eC9BP0QK", "aGVsbG93b3JsZGxvbmcuLgo=")
	require.Error(t, err)
}

func TestAesEcbBase64(t *testing.T) {
	const (
		val     = "hello"
		badKey1 = "aaaaaaaaa"
		// more than 32 chars
		badKey2 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	key := []byte("q4t7w!z%C*F-JaNdRgUjXn2r5u8x/A?D")
	b64Key := base64.StdEncoding.EncodeToString(key)
	b64Val := base64.StdEncoding.EncodeToString([]byte(val))
	_, err := EcbEncryptBase64(badKey1, val)
	require.Error(t, err)
	_, err = EcbEncryptBase64(badKey2, val)
	require.Error(t, err)
	_, err = EcbEncryptBase64(b64Key, val)
	require.Error(t, err)
	dst, err := EcbEncryptBase64(b64Key, b64Val)
	require.NoError(t, err)
	_, err = EcbDecryptBase64(badKey1, dst)
	require.Error(t, err)
	_, err = EcbDecryptBase64(badKey2, dst)
	require.Error(t, err)
	_, err = EcbDecryptBase64(b64Key, val)
	require.Error(t, err)
	src, err := EcbDecryptBase64(b64Key, dst)
	require.NoError(t, err)
	b, err := base64.StdEncoding.DecodeString(src)
	require.NoError(t, err)
	assert.Equal(t, val, string(b))
}

func TestPkcs5UnpaddingEmptyInput(t *testing.T) {
	_, err := pkcs5Unpadding([]byte{}, 16)
	assert.Equal(t, ErrPaddingSize, err)
}

func TestPkcs5UnpaddingMalformedPadding(t *testing.T) {
	// Valid PKCS5 padding of 3: last 3 bytes should all be 0x03
	// Here we corrupt one padding byte
	malformed := []byte{
		0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41,
		0x41, 0x41, 0x41, 0x41, 0x41, 0x02, 0x03, 0x03,
	}
	_, err := pkcs5Unpadding(malformed, 16)
	assert.Equal(t, ErrPaddingSize, err)

	// All padding bytes correct
	valid := []byte{
		0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41,
		0x41, 0x41, 0x41, 0x41, 0x41, 0x03, 0x03, 0x03,
	}
	result, err := pkcs5Unpadding(valid, 16)
	require.NoError(t, err)
	assert.Equal(t, valid[:13], result)
}

func TestPkcs5UnpaddingInvalidPaddingValue(t *testing.T) {
	// padding value = 0 (< 1)
	_, err := pkcs5Unpadding([]byte{0x41, 0x00}, 16)
	assert.Equal(t, ErrPaddingSize, err)

	// padding value > blockSize
	_, err = pkcs5Unpadding([]byte{0x41, 0x41, 0x41, 0x41, 17}, 4)
	assert.Equal(t, ErrPaddingSize, err)

	// padding value > length
	_, err = pkcs5Unpadding([]byte{0x41, 0x03}, 16)
	assert.Equal(t, ErrPaddingSize, err)
}

func TestEcbDecryptEmptyInput(t *testing.T) {
	key := []byte("q4t7w!z%C*F-JaNdRgUjXn2r5u8x/A?D")
	_, err := EcbDecrypt(key, []byte{})
	assert.Equal(t, ErrPaddingSize, err)
}
