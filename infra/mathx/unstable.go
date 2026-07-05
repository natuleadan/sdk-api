package mathx

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

// An Unstable is used to generate random value around the mean value based on given deviation.
type Unstable struct {
	deviation float64
	lock      *sync.Mutex
	buf       [8]byte
}

// NewUnstable returns an Unstable.
func NewUnstable(deviation float64) Unstable {
	if deviation < 0 {
		deviation = 0
	}
	if deviation > 1 {
		deviation = 1
	}
	return Unstable{
		deviation: deviation,
		lock:      new(sync.Mutex),
	}
}

func (u *Unstable) readFloat64() float64 {
	if _, err := rand.Read(u.buf[:]); err != nil {
		return 0
	}
	return float64(binary.LittleEndian.Uint64(u.buf[:])) / float64(1<<64)
}

// AroundDuration returns a random duration with given base and deviation.
func (u Unstable) AroundDuration(base time.Duration) time.Duration {
	u.lock.Lock()
	defer u.lock.Unlock()
	return time.Duration((1 + u.deviation - 2*u.deviation*u.readFloat64()) * float64(base))
}

// AroundInt returns a random int64 with given base and deviation.
func (u Unstable) AroundInt(base int64) int64 {
	u.lock.Lock()
	defer u.lock.Unlock()
	return int64((1 + u.deviation - 2*u.deviation*u.readFloat64()) * float64(base))
}
