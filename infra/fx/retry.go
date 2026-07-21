package fx

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	"github.com/natuleadan/sdk-api/infra/errorx"
)

const defaultRetryTimes = 3

type (
	RetryOption func(*retryOptions)

	retryOptions struct {
		times            int
		interval         time.Duration
		backoffInitial   time.Duration
		backoffMax       time.Duration
		backoffMultipler float64
		timeout          time.Duration
		ignoreErrors     []error
	}
)

func DoWithRetry(fn func() error, opts ...RetryOption) error {
	return retry(context.Background(), func(errChan chan error, retryCount int) {
		errChan <- fn()
	}, opts...)
}

func DoWithRetryCtx(ctx context.Context, fn func(ctx context.Context, retryCount int) error,
	opts ...RetryOption,
) error {
	return retry(ctx, func(errChan chan error, retryCount int) {
		errChan <- fn(ctx, retryCount)
	}, opts...)
}

func retry(ctx context.Context, fn func(errChan chan error, retryCount int), opts ...RetryOption) error {
	options := newRetryOptions()
	for _, opt := range opts {
		opt(options)
	}

	var berr errorx.BatchError
	var cancelFunc context.CancelFunc
	if options.timeout > 0 {
		ctx, cancelFunc = context.WithTimeout(ctx, options.timeout)
		defer cancelFunc()
	}

	errChan := make(chan error, 1)
	for i := 0; i < options.times; i++ {
		go fn(errChan, i)

		select {
		case err := <-errChan:
			if err != nil {
				for _, ignoreErr := range options.ignoreErrors {
					if errors.Is(err, ignoreErr) {
						return nil
					}
				}
				berr.Add(err)
			} else {
				return nil
			}
		case <-ctx.Done():
			berr.Add(ctx.Err())
			return berr.Err()
		}

		if i < options.times-1 {
			wait := options.interval
			if options.backoffInitial > 0 {
				wait = nextBackoff(i, options.backoffInitial, options.backoffMax, options.backoffMultipler)
			}
			if wait > 0 {
				select {
				case <-ctx.Done():
					berr.Add(ctx.Err())
					return berr.Err()
				case <-time.After(wait):
				}
			}
		}
	}

	return berr.Err()
}

func nextBackoff(attempt int, initial, max time.Duration, multiplier float64) time.Duration {
	d := float64(initial)
	for range attempt {
		d *= multiplier
		if d >= float64(max) {
			return max
		}
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		return max
	}
	d += float64(n.Int64()) * float64(time.Millisecond)
	if d >= float64(max) {
		return max
	}
	return time.Duration(d)
}

func WithIgnoreErrors(ignoreErrors []error) RetryOption {
	return func(options *retryOptions) {
		options.ignoreErrors = ignoreErrors
	}
}

func WithInterval(interval time.Duration) RetryOption {
	return func(options *retryOptions) {
		options.interval = interval
	}
}

func WithRetry(times int) RetryOption {
	return func(options *retryOptions) {
		options.times = times
	}
}

func WithTimeout(timeout time.Duration) RetryOption {
	return func(options *retryOptions) {
		options.timeout = timeout
	}
}

func WithBackoff(initial, max time.Duration, multiplier float64) RetryOption {
	return func(options *retryOptions) {
		options.backoffInitial = initial
		options.backoffMax = max
		options.backoffMultipler = multiplier
	}
}

func newRetryOptions() *retryOptions {
	return &retryOptions{
		times: defaultRetryTimes,
	}
}
