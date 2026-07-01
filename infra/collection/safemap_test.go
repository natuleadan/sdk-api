package collection

import (
	"sync/atomic"
	"testing"

	"github.com/natuleadan/sdk-api/infra/stringx"
	"github.com/stretchr/testify/assert"
)

func TestSafeMap(t *testing.T) {
	tests := []struct {
		size      int
		exception int
	}{
		{
			100000,
			2000,
		},
		{
			100000,
			50,
		},
	}
	for _, test := range tests {
		t.Run(stringx.Rand(), func(t *testing.T) {
			testSafeMapWithParameters(t, test.size, test.exception)
		})
	}
}

func TestSafeMap_CopyNew(t *testing.T) {
	const (
		size       = 100000
		exception1 = 5
		exception2 = 500
	)
	m := NewSafeMap()

	for i := range size {
		m.Set(i, i)
	}
	for i := range size {
		if i%exception1 == 0 {
			m.Del(i)
		}
	}

	for i := size; i < size<<1; i++ {
		m.Set(i, i)
	}
	for i := size; i < size<<1; i++ {
		if i%exception2 != 0 {
			m.Del(i)
		}
	}

	for i := range size {
		val, ok := m.Get(i)
		if i%exception1 != 0 {
			assert.True(t, ok)
			assert.Equal(t, i, val.(int))
		} else {
			assert.False(t, ok)
		}
	}
	for i := size; i < size<<1; i++ {
		val, ok := m.Get(i)
		if i%exception2 == 0 {
			assert.True(t, ok)
			assert.Equal(t, i, val.(int))
		} else {
			assert.False(t, ok)
		}
	}
}

func testSafeMapWithParameters(t *testing.T, size, exception int) {
	m := NewSafeMap()

	for i := range size {
		m.Set(i, i)
	}
	for i := range size {
		if i%exception != 0 {
			m.Del(i)
		}
	}

	assert.Equal(t, size/exception, m.Size())

	for i := size; i < size<<1; i++ {
		m.Set(i, i)
	}
	for i := size; i < size<<1; i++ {
		if i%exception != 0 {
			m.Del(i)
		}
	}

	for i := 0; i < size<<1; i++ {
		val, ok := m.Get(i)
		if i%exception == 0 {
			assert.True(t, ok)
			assert.Equal(t, i, val.(int))
		} else {
			assert.False(t, ok)
		}
	}
}

func TestSafeMap_Range(t *testing.T) {
	const (
		size       = 100000
		exception1 = 5
		exception2 = 500
	)

	m := NewSafeMap()
	newMap := NewSafeMap()

	for i := range size {
		m.Set(i, i)
	}
	for i := range size {
		if i%exception1 == 0 {
			m.Del(i)
		}
	}

	for i := size; i < size<<1; i++ {
		m.Set(i, i)
	}
	for i := size; i < size<<1; i++ {
		if i%exception2 != 0 {
			m.Del(i)
		}
	}

	var count int32
	m.Range(func(k, v any) bool {
		atomic.AddInt32(&count, 1)
		newMap.Set(k, v)
		return true
	})
	assert.Equal(t, int(atomic.LoadInt32(&count)), m.Size())
	assert.Equal(t, m.dirtyNew, newMap.dirtyNew)
	assert.Equal(t, m.dirtyOld, newMap.dirtyOld)
}

func TestSetManyTimes(t *testing.T) {
	const iteration = maxDeletion * 2
	m := NewSafeMap()
	for i := range iteration {
		m.Set(i, i)
		if i%3 == 0 {
			m.Del(i / 2)
		}
	}
	var count int
	m.Range(func(k, v any) bool {
		count++
		return count < maxDeletion/2
	})
	assert.Equal(t, maxDeletion/2, count)
	for i := range iteration {
		m.Set(i, i)
		if i%3 == 0 {
			m.Del(i / 2)
		}
	}
	for i := range iteration {
		m.Set(i, i)
		if i%3 == 0 {
			m.Del(i / 2)
		}
	}
	for i := range iteration {
		m.Set(i, i)
		if i%3 == 0 {
			m.Del(i / 2)
		}
	}

	count = 0
	m.Range(func(k, v any) bool {
		count++
		return count < maxDeletion
	})
	assert.Equal(t, maxDeletion, count)
}

func TestSetManyTimesNew(t *testing.T) {
	m := NewSafeMap()
	for i := range maxDeletion * 3 {
		m.Set(i, i)
	}
	for i := range maxDeletion * 2 {
		m.Del(i)
	}
	for i := range maxDeletion * 3 {
		m.Set(i+maxDeletion*3, i+maxDeletion*3)
	}
	for i := range maxDeletion * 2 {
		m.Del(i + maxDeletion*2)
	}
	for i := range maxDeletion - copyThreshold + 1 {
		m.Del(i + maxDeletion*2)
	}
	assert.Equal(t, 0, len(m.dirtyNew))
}
