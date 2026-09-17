package executors

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDelayExecutor(t *testing.T) {
	var count atomic.Int32
	ex := NewDelayExecutor(func() {
		count.Add(1)
	}, time.Millisecond*10)
	for range 100 {
		ex.Trigger()
	}
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, int32(1), count.Load())
}
