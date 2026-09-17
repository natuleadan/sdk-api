package rescue

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/stretchr/testify/assert"
)

func init() {
	logx.Disable()
}

func TestRescue(t *testing.T) {
	var count atomic.Int32
	assert.NotPanics(t, func() {
		defer Recover(func() {
			count.Add(2)
		}, func() {
			count.Add(3)
		})

		panic("hello")
	})
	assert.Equal(t, int32(5), count.Load())
}

func TestRescueCtx(t *testing.T) {
	var count atomic.Int32
	assert.NotPanics(t, func() {
		defer RecoverCtx(context.Background(), func() {
			count.Add(2)
		}, func() {
			count.Add(3)
		})

		panic("hello")
	})
	assert.Equal(t, int32(5), count.Load())
}
