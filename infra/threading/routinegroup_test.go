package threading

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/natuleadan/sdk-api/infra/logx/logtest"
	"github.com/stretchr/testify/assert"
)

func TestRoutineGroupRun(t *testing.T) {
	var count int32
	group := NewRoutineGroup()
	for range 3 {
		group.Run(func() {
			atomic.AddInt32(&count, 1)
		})
	}

	group.Wait()

	assert.Equal(t, int32(3), count)
}

func TestRoutingGroupRunSafe(t *testing.T) {
	logtest.Discard(t)

	var count int32
	group := NewRoutineGroup()
	var once sync.Once
	for range 3 {
		group.RunSafe(func() {
			once.Do(func() {
				panic("")
			})
			atomic.AddInt32(&count, 1)
		})
	}

	group.Wait()

	assert.Equal(t, int32(2), count)
}
