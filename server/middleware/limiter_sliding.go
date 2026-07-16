package middleware

import (
	"math"
	"sync"
	"time"
)

type slidingWindowLimiter struct {
	mu         sync.Mutex
	prevHits   int
	currHits   int
	windowExp  int64
	max        int
	expiration int64
}

func newSlidingWindowLimiter(max, burst int) *slidingWindowLimiter {
	if burst <= 0 {
		burst = max
	}
	return &slidingWindowLimiter{
		max:        max,
		expiration: int64(burst),
	}
}

func (l *slidingWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().Unix()

	// First request: initialize window
	if l.windowExp == 0 {
		l.windowExp = now + l.expiration
		l.currHits = 1
		return true
	}

	// Rotate window if needed
	if now >= l.windowExp {
		elapsed := now - (l.windowExp - l.expiration)
		if elapsed >= l.expiration {
			// Fully expired: reset both counters
			l.prevHits = 0
			l.currHits = 0
			l.windowExp = now + l.expiration
		} else {
			// Partially expired: slide currHits → prevHits
			l.prevHits = l.currHits
			l.currHits = 0
			l.windowExp += l.expiration
		}
	}

	// Calculate weighted rate using sliding window formula
	elapsed := l.windowExp - now
	weight := float64(elapsed) / float64(l.expiration)
	rate := float64(l.prevHits)*(1-weight) + float64(l.currHits)

	if rate >= float64(l.max) {
		return false
	}

	l.currHits++
	return true
}

func (l *slidingWindowLimiter) Remaining() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().Unix()

	if l.windowExp == 0 {
		return l.max
	}

	if now >= l.windowExp {
		return l.max
	}

	elapsed := l.windowExp - now
	weight := float64(elapsed) / float64(l.expiration)
	rate := float64(l.prevHits)*(1-weight) + float64(l.currHits)

	remaining := l.max - int(math.Ceil(rate))
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (l *slidingWindowLimiter) Cancel() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.currHits > 0 {
		l.currHits--
	}
}
