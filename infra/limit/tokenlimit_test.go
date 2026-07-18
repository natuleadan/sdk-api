package limit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/natuleadan/sdk-api/infra/stores/redis/redistest"
	"github.com/stretchr/testify/assert"
)

func init() {
	logx.Disable()
}

func TestTokenLimit_WithCtx(t *testing.T) {
	s, err := miniredis.Run()
	assert.NoError(t, err)

	const (
		total = 100
		rate  = 5
		burst = 10
	)
	l := NewTokenLimiter(rate, burst, redis.MustNewRedis(redis.RedisConf{Host: s.Addr(), Type: redis.NodeType}), "tokenlimit")
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ok := l.AllowCtx(ctx)
	assert.True(t, ok)

	cancel()
	for range total {
		ok := l.AllowCtx(ctx)
		assert.False(t, ok)
		assert.False(t, l.monitorStarted)
	}
}

func TestTokenLimit_Rescue(t *testing.T) {
	s, err := miniredis.Run()
	assert.NoError(t, err)

	const (
		total = 100
		rate  = 5
		burst = 10
	)
	l := NewTokenLimiter(rate, burst, redis.MustNewRedis(redis.RedisConf{Host: s.Addr(), Type: redis.NodeType}), "tokenlimit")
	s.Close()

	var allowed int
	for i := range total {
		time.Sleep(time.Second / time.Duration(total))
		if i == total>>1 {
			assert.NoError(t, s.Restart())
		}
		if l.Allow() {
			allowed++
		}

		// make sure start monitor more than once doesn't matter
		l.startMonitor()
	}

	assert.GreaterOrEqual(t, allowed, burst+rate)
}

func TestTokenLimit_Take(t *testing.T) {
	store := redistest.CreateRedis(t)

	const (
		total = 100
		rate  = 5
		burst = 10
	)
	l := NewTokenLimiter(rate, burst, store, "tokenlimit")
	var allowed int
	for range total {
		time.Sleep(time.Second / time.Duration(total))
		if l.Allow() {
			allowed++
		}
	}

	assert.GreaterOrEqual(t, allowed, burst+rate)
}

func TestTokenLimit_TakeBurst(t *testing.T) {
	store := redistest.CreateRedis(t)

	const (
		total = 100
		rate  = 5
		burst = 10
	)
	l := NewTokenLimiter(rate, burst, store, "tokenlimit")
	var allowed int
	for range total {
		if l.Allow() {
			allowed++
		}
	}

	assert.GreaterOrEqual(t, allowed, burst)
}
