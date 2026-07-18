package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/natuleadan/sdk-api/infra/breaker"
	"github.com/stretchr/testify/assert"
)

func TestBreakerHook_ProcessHook(t *testing.T) {
	t.Run("breakerHookOpen", func(t *testing.T) {
		s := miniredis.RunT(t)

		rds := MustNewRedis(RedisConf{
			Host: s.Addr(),
			Type: NodeType,
		})

		someError := errors.New("ERR some error")
		s.SetError(someError.Error())

		var err error
		for range 1000 {
			_, err = rds.Get("key")
			if err != nil && err.Error() != someError.Error() {
				break
			}
		}
		assert.Equal(t, breaker.ErrServiceUnavailable, err)
	})

	t.Run("breakerHookClose", func(t *testing.T) {
		s := miniredis.RunT(t)

		rds := MustNewRedis(RedisConf{
			Host: s.Addr(),
			Type: NodeType,
		})

		var err error
		for range 1000 {
			_, err = rds.Get("key")
			if err != nil {
				break
			}
		}
		assert.NotEqual(t, breaker.ErrServiceUnavailable, err)
	})

	t.Run("breakerHook_ignoreCmd", func(t *testing.T) {
		s := miniredis.RunT(t)

		rds := MustNewRedis(RedisConf{
			Host: s.Addr(),
			Type: NodeType,
		})

		someError := errors.New("ERR some error")
		s.SetError(someError.Error())

		var err error

		node, err := getRedis(rds)
		assert.NoError(t, err)

		for range 1000 {
			_, err = rds.Blpop(node, "key")
			if err != nil && err.Error() != someError.Error() {
				break
			}
		}
		assert.Equal(t, someError.Error(), err.Error())
	})
}

func TestBreakerHook_ProcessPipelineHook(t *testing.T) {
	t.Run("breakerPipelineHookOpen", func(t *testing.T) {
		s := miniredis.RunT(t)

		rds := MustNewRedis(RedisConf{
			Host: s.Addr(),
			Type: NodeType,
		})

		someError := errors.New("ERR some error")
		s.SetError(someError.Error())

		var err error
		for range 1000 {
			err = rds.Pipelined(
				func(pipe Pipeliner) error {
					pipe.Incr(context.Background(), "pipelined_counter")
					pipe.Expire(context.Background(), "pipelined_counter", time.Hour)
					pipe.ZAdd(context.Background(), "zadd", Z{Score: 12, Member: "zadd"})
					return nil
				},
			)

			if err != nil && err.Error() != someError.Error() {
				break
			}
		}
		assert.Equal(t, breaker.ErrServiceUnavailable, err)
	})

	t.Run("breakerPipelineHookClose", func(t *testing.T) {
		s := miniredis.RunT(t)

		rds := MustNewRedis(RedisConf{
			Host: s.Addr(),
			Type: NodeType,
		})

		var err error
		for range 1000 {
			err = rds.Pipelined(
				func(pipe Pipeliner) error {
					pipe.Incr(context.Background(), "pipelined_counter")
					pipe.Expire(context.Background(), "pipelined_counter", time.Hour)
					pipe.ZAdd(context.Background(), "zadd", Z{Score: 12, Member: "zadd"})
					return nil
				},
			)
			if err != nil {
				break
			}
		}
		assert.NotEqual(t, breaker.ErrServiceUnavailable, err)
	})
}
