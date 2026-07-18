package mathx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUnstable_AroundDuration(t *testing.T) {
	unstable := NewUnstable(0.05)
	for range 1000 {
		val := unstable.AroundDuration(time.Second)
		assert.LessOrEqual(t, float64(time.Second)*0.95, float64(val))
		assert.LessOrEqual(t, float64(val), float64(time.Second)*1.05)
	}
}

func TestUnstable_AroundInt(t *testing.T) {
	const target = 10000
	unstable := NewUnstable(0.05)
	for range 1000 {
		val := unstable.AroundInt(target)
		assert.LessOrEqual(t, float64(target)*0.95, float64(val))
		assert.LessOrEqual(t, float64(val), float64(target)*1.05)
	}
}

func TestUnstable_AroundIntLarge(t *testing.T) {
	const target int64 = 10000
	unstable := NewUnstable(5)
	for range 1000 {
		val := unstable.AroundInt(target)
		assert.LessOrEqual(t, int64(0), val)
		assert.LessOrEqual(t, val, 2*target)
	}
}

func TestUnstable_AroundIntNegative(t *testing.T) {
	const target int64 = 10000
	unstable := NewUnstable(-0.05)
	for range 1000 {
		val := unstable.AroundInt(target)
		assert.Equal(t, target, val)
	}
}

func TestUnstable_Distribution(t *testing.T) {
	const (
		seconds = 10000
		total   = 10000
	)

	m := make(map[int]int)
	expiry := NewUnstable(0.05)
	for range total {
		val := int(expiry.AroundInt(seconds))
		m[val]++
	}

	_, ok := m[0]
	assert.False(t, ok)

	mi := make(map[any]int, len(m))
	for k, v := range m {
		mi[k] = v
	}
	entropy := CalcEntropy(mi)
	assert.Greater(t, len(m), 1)
	assert.Greater(t, entropy, 0.95)
}
