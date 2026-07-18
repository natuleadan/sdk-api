package breaker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNopBreaker(t *testing.T) {
	b := NopBreaker()
	assert.Equal(t, nopBreakerName, b.Name())
	_, err := b.Allow()
	require.NoError(t, err)
	p, err := b.AllowCtx(context.Background())
	require.NoError(t, err)
	p.Accept()
	for range 1000 {
		p, err := b.Allow()
		require.NoError(t, err)
		p.Reject("any")
	}
	assert.NoError(t, b.Do(func() error {
		return nil
	}))
	assert.NoError(t, b.DoCtx(context.Background(), func() error {
		return nil
	}))
	assert.NoError(t, b.DoWithAcceptable(func() error {
		return nil
	}, defaultAcceptable))
	assert.NoError(t, b.DoWithAcceptableCtx(context.Background(), func() error {
		return nil
	}, defaultAcceptable))
	errDummy := errors.New("any")
	assert.Equal(t, errDummy, b.DoWithFallback(func() error {
		return errDummy
	}, func(err error) error {
		return nil
	}))
	assert.Equal(t, errDummy, b.DoWithFallbackCtx(context.Background(), func() error {
		return errDummy
	}, func(err error) error {
		return nil
	}))
	assert.Equal(t, errDummy, b.DoWithFallbackAcceptable(func() error {
		return errDummy
	}, func(err error) error {
		return nil
	}, defaultAcceptable))
	assert.Equal(t, errDummy, b.DoWithFallbackAcceptableCtx(context.Background(), func() error {
		return errDummy
	}, func(err error) error {
		return nil
	}, defaultAcceptable))
}
