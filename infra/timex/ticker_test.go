package timex

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRealTickerDoTick(t *testing.T) {
	ticker := NewTicker(time.Millisecond * 10)
	defer ticker.Stop()
	var count int
	for range ticker.Chan() {
		count++
		if count > 5 {
			break
		}
	}
}

func TestFakeTicker(t *testing.T) {
	const total = 5
	ticker := NewFakeTicker()
	defer ticker.Stop()

	var count atomic.Int32
	go func() {
		for range ticker.Chan() {
			if count.Add(1) == total {
				ticker.Done()
			}
		}
	}()

	for range 5 {
		ticker.Tick()
	}

	assert.NoError(t, ticker.Wait(time.Second))
	assert.Equal(t, int32(total), count.Load())
}

func TestFakeTickerTimeout(t *testing.T) {
	ticker := NewFakeTicker()
	defer ticker.Stop()

	assert.Error(t, ticker.Wait(time.Millisecond))
}
