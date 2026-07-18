package syncx

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeoutLimit(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
	}{
		{
			name: "no wait",
		},
		{
			name:     "wait",
			interval: time.Millisecond * 100,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limit := NewTimeoutLimit(2)
			assert.NoError(t, limit.Borrow(time.Millisecond*200))
			assert.NoError(t, limit.Borrow(time.Millisecond*200))
			var wait1, wait2, wait3 sync.WaitGroup
			wait1.Add(1)
			wait2.Add(1)
			wait3.Go(func() {
				wait1.Wait()
				wait2.Done()
				time.Sleep(test.interval)
				assert.NoError(t, limit.Return())
			})
			wait1.Done()
			wait2.Wait()
			require.NoError(t, limit.Borrow(time.Second))
			wait3.Wait()
			assert.Equal(t, ErrTimeout, limit.Borrow(time.Millisecond*100))
			assert.NoError(t, limit.Return())
			assert.NoError(t, limit.Return())
			assert.Equal(t, ErrLimitReturn, limit.Return())
		})
	}
}
