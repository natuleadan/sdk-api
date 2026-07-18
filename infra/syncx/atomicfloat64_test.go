package syncx

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtomicFloat64(t *testing.T) {
	f := ForAtomicFloat64(100)
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			for range 100 {
				f.Add(1)
			}
		})
	}
	wg.Wait()
	assert.InDelta(t, float64(600), f.Load(), 0.01)
}
