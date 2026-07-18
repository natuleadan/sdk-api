package threading

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRunner_Schedule(t *testing.T) {
	times := 100
	pool := NewTaskRunner(runtime.NumCPU())

	var counter int32
	for range times {
		pool.Schedule(func() {
			atomic.AddInt32(&counter, 1)
		})
	}

	pool.Wait()

	assert.Equal(t, times, int(counter))
}

func TestTaskRunner_ScheduleImmediately(t *testing.T) {
	cpus := runtime.NumCPU()
	times := cpus * 2
	pool := NewTaskRunner(cpus)

	var counter int32
	for i := range times {
		err := pool.ScheduleImmediately(func() {
			atomic.AddInt32(&counter, 1)
			time.Sleep(time.Millisecond * 100)
		})
		if i < cpus {
			require.NoError(t, err)
		} else {
			assert.ErrorIs(t, err, ErrTaskRunnerBusy)
		}
	}

	pool.Wait()

	assert.Equal(t, cpus, int(counter))
}

func BenchmarkRoutinePool(b *testing.B) {
	queue := NewTaskRunner(runtime.NumCPU())
	for i := 0; i < b.N; i++ {
		queue.Schedule(func() {
		})
	}
}
