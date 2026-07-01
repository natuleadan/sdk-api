package hash

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/spaolacci/murmur3"
)

// Hash returns the hash value of data.
func Hash(data []byte) uint64 {
	return murmur3.Sum64(data)
}

// Md5 returns the sha256 bytes of data.
func Md5(data []byte) []byte {
	digest := sha256.New()
	digest.Write(data)
	return digest.Sum(nil)
}

// Md5Hex returns the sha256 hex string of data.
func Md5Hex(data []byte) string {
	return hex.EncodeToString(Md5(data))
}
