package breaker

import (
	"context"
	"errors"
	"testing"

	"github.com/natuleadan/sdk-api/infra/stat"
	"github.com/stretchr/testify/assert"
)

func init() {
	stat.SetReporter(nil)
}

func TestBreakersDo(t *testing.T) {
	assert.NoError(t, Do("any", func() error {
		return nil
	}))

	errDummy := errors.New("any")
	assert.Equal(t, errDummy, Do("any", func() error {
		return errDummy
	}))
	assert.Equal(t, errDummy, DoCtx(context.Background(), "any", func() error {
		return errDummy
	}))
}

func TestBreakersDoWithAcceptable(t *testing.T) {
	errDummy := errors.New("anyone")
	for range 10000 {
		assert.Equal(t, errDummy, GetBreaker("anyone").DoWithAcceptable(func() error {
			return errDummy
		}, func(err error) bool {
			return err == nil || errors.Is(err, errDummy)
		}))
	}
	verify(t, func() bool {
		return Do("anyone", func() error {
			return nil
		}) == nil
	})
	verify(t, func() bool {
		return DoWithAcceptableCtx(context.Background(), "anyone", func() error {
			return nil
		}, func(err error) bool {
			return true
		}) == nil
	})

	for range 10000 {
		err := DoWithAcceptable("another", func() error {
			return errDummy
		}, func(err error) bool {
			return err == nil
		})
		assert.True(t, errors.Is(err, errDummy) || errors.Is(err, ErrServiceUnavailable))
	}
	verify(t, func() bool {
		return errors.Is(Do("another", func() error {
			return nil
		}), ErrServiceUnavailable)
	})
}

func TestBreakersNoBreakerFor(t *testing.T) {
	NoBreakerFor("any")
	errDummy := errors.New("any")
	for range 10000 {
		assert.Equal(t, errDummy, GetBreaker("any").Do(func() error {
			return errDummy
		}))
	}
	assert.NoError(t, Do("any", func() error {
		return nil
	}))
}

func TestBreakersFallback(t *testing.T) {
	errDummy := errors.New("any")
	for range 10000 {
		err := DoWithFallback("fallback", func() error {
			return errDummy
		}, func(err error) error {
			return nil
		})
		assert.True(t, err == nil || errors.Is(err, errDummy))
		err = DoWithFallbackCtx(context.Background(), "fallback", func() error {
			return errDummy
		}, func(err error) error {
			return nil
		})
		assert.True(t, err == nil || errors.Is(err, errDummy))
	}
	verify(t, func() bool {
		return errors.Is(Do("fallback", func() error {
			return nil
		}), ErrServiceUnavailable)
	})
}

func TestBreakersAcceptableFallback(t *testing.T) {
	errDummy := errors.New("any")
	for range 5000 {
		err := DoWithFallbackAcceptable("acceptablefallback", func() error {
			return errDummy
		}, func(err error) error {
			return nil
		}, func(err error) bool {
			return err == nil
		})
		assert.True(t, err == nil || errors.Is(err, errDummy))
		err = DoWithFallbackAcceptableCtx(context.Background(), "acceptablefallback", func() error {
			return errDummy
		}, func(err error) error {
			return nil
		}, func(err error) bool {
			return err == nil
		})
		assert.True(t, err == nil || errors.Is(err, errDummy))
	}
	verify(t, func() bool {
		return errors.Is(Do("acceptablefallback", func() error {
			return nil
		}), ErrServiceUnavailable)
	})
}

func verify(t *testing.T, fn func() bool) {
	var count int
	for range 100 {
		if fn() {
			count++
		}
	}
	assert.GreaterOrEqual(t, count, 75, "should be greater than 75, actual %d", count)
}
