package stringx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRand(t *testing.T) {
	Seed(time.Now().UnixNano())
	assert.NotEmpty(t, Rand())
	assert.NotEmpty(t, RandId())

	const size = 10
	assert.Len(t, Randn(size), size)
}

func BenchmarkRandString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Randn(10)
	}
}
