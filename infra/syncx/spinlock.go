package syncx

import (
	"runtime"
	"sync/atomic"
)

// A SpinLock is used as a lock a fast execution.
type SpinLock struct {
	lock atomic.Uint32
}

// Lock locks the SpinLock.
func (sl *SpinLock) Lock() {
	for !sl.TryLock() {
		runtime.Gosched()
	}
}

// TryLock tries to lock the SpinLock.
func (sl *SpinLock) TryLock() bool {
	return sl.lock.CompareAndSwap(0, 1)
}

// Unlock unlocks the SpinLock.
func (sl *SpinLock) Unlock() {
	sl.lock.Store(0)
}
