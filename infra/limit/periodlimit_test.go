package limit

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/natuleadan/sdk-api/infra/stores/redis/redistest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeriodLimit_Take(t *testing.T) {
	testPeriodLimit(t)
}

func TestPeriodLimit_TakeWithAlign(t *testing.T) {
	testPeriodLimit(t, Align())
}

func TestPeriodLimit_RedisUnavailable(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)

	const (
		seconds = 1
		quota   = 5
	)
	l := NewPeriodLimit(seconds, quota, redis.MustNewRedis(redis.RedisConf{Host: s.Addr(), Type: redis.NodeType}), "periodlimit")
	s.Close()
	val, err := l.Take("first")
	require.Error(t, err)
	assert.InDelta(t, 0, val, 0.01)
}

func testPeriodLimit(t *testing.T, opts ...PeriodOption) {
	store := redistest.CreateRedis(t)

	const (
		seconds = 1
		total   = 100
		quota   = 5
	)
	l := NewPeriodLimit(seconds, quota, store, "periodlimit", opts...)
	var allowed, hitQuota, overQuota int
	for range total {
		val, err := l.Take("first")
		if err != nil {
			t.Error(err)
		}
		switch val {
		case Allowed:
			allowed++
		case HitQuota:
			hitQuota++
		case OverQuota:
			overQuota++
		default:
			t.Error("unknown status")
		}
	}

	assert.Equal(t, quota-1, allowed)
	assert.InDelta(t, 1, hitQuota, 0.01)
	assert.Equal(t, total-quota, overQuota)
}

func TestQuotaFull(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)

	l := NewPeriodLimit(1, 1, redis.MustNewRedis(redis.RedisConf{Host: s.Addr(), Type: redis.NodeType}), "periodlimit")
	val, err := l.Take("first")
	require.NoError(t, err)
	assert.Equal(t, HitQuota, val)
}
