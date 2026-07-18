package redis

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/natuleadan/sdk-api/infra/breaker"
	"github.com/natuleadan/sdk-api/infra/logx/logtest"
	"github.com/natuleadan/sdk-api/infra/trace/tracetest"
	red "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	tracesdk "go.opentelemetry.io/otel/trace"
)

func TestHookProcessCase1(t *testing.T) {
	tracetest.NewInMemoryExporter(t)
	w := logtest.NewCollector(t)

	err := defaultDurationHook.ProcessHook(func(ctx context.Context, cmd red.Cmder) error {
		assert.Equal(t, "redis", tracesdk.SpanFromContext(ctx).(interface{ Name() string }).Name())
		return nil
	})(context.Background(), red.NewCmd(context.Background()))
	if err != nil {
		t.Fatal(err)
	}

	assert.NotContains(t, w.String(), "slow")
}

func TestHookProcessCase2(t *testing.T) {
	tracetest.NewInMemoryExporter(t)
	w := logtest.NewCollector(t)

	err := defaultDurationHook.ProcessHook(func(ctx context.Context, cmd red.Cmder) error {
		assert.Equal(t, "redis", tracesdk.SpanFromContext(ctx).(interface{ Name() string }).Name())
		time.Sleep(slowThreshold.Load() + time.Millisecond)
		return nil
	})(context.Background(), red.NewCmd(context.Background()))
	if err != nil {
		t.Fatal(err)
	}

	assert.Contains(t, w.String(), "slow")
	assert.Contains(t, w.String(), "trace")
	assert.Contains(t, w.String(), "span")
}

func TestHookProcessPipelineCase1(t *testing.T) {
	tracetest.NewInMemoryExporter(t)
	w := logtest.NewCollector(t)

	err := defaultDurationHook.ProcessPipelineHook(func(ctx context.Context, cmds []red.Cmder) error {
		return nil
	})(context.Background(), nil)
	assert.NoError(t, err)

	err = defaultDurationHook.ProcessPipelineHook(func(ctx context.Context, cmds []red.Cmder) error {
		assert.Equal(t, "redis", tracesdk.SpanFromContext(ctx).(interface{ Name() string }).Name())
		return nil
	})(context.Background(), []red.Cmder{
		red.NewCmd(context.Background()),
	})
	assert.NoError(t, err)

	assert.NotContains(t, w.String(), "slow")
}

func TestHookProcessPipelineCase2(t *testing.T) {
	tracetest.NewInMemoryExporter(t)
	w := logtest.NewCollector(t)

	err := defaultDurationHook.ProcessPipelineHook(func(ctx context.Context, cmds []red.Cmder) error {
		assert.Equal(t, "redis", tracesdk.SpanFromContext(ctx).(interface{ Name() string }).Name())
		time.Sleep(slowThreshold.Load() + time.Millisecond)
		return nil
	})(context.Background(), []red.Cmder{
		red.NewCmd(context.Background()),
	})
	assert.NoError(t, err)

	assert.Contains(t, w.String(), "slow")
	assert.Contains(t, w.String(), "trace")
	assert.Contains(t, w.String(), "span")
}

func TestHookProcessPipelineCase3(t *testing.T) {
	te := tracetest.NewInMemoryExporter(t)

	err := defaultDurationHook.ProcessPipelineHook(func(ctx context.Context, cmds []red.Cmder) error {
		assert.Equal(t, "redis", tracesdk.SpanFromContext(ctx).(interface{ Name() string }).Name())
		return assert.AnError
	})(context.Background(), []red.Cmder{
		red.NewCmd(context.Background()),
	})
	assert.ErrorIs(t, err, assert.AnError)
	traceLogs := te.GetSpans().Snapshots()[0]
	assert.Equal(t, "redis", traceLogs.Name())
	assert.Equal(t, assert.AnError.Error(), traceLogs.Events()[0].Attributes[1].Value.AsString(), "trace should record error")
}

func TestLogDuration(t *testing.T) {
	w := logtest.NewCollector(t)

	logDuration(context.Background(), []red.Cmder{
		red.NewCmd(context.Background(), "get", "foo"),
	}, 1*time.Second)
	assert.Contains(t, w.String(), "get foo")

	logDuration(context.Background(), []red.Cmder{
		red.NewCmd(context.Background(), "get", "foo"),
		red.NewCmd(context.Background(), "set", "bar", 0),
	}, 1*time.Second)
	assert.Contains(t, w.String(), `get foo\nset bar 0`)
}

func TestFormatError(t *testing.T) {
	// Test case: err is OpError
	err := &net.OpError{
		Err: mockOpError{},
	}
	assert.Equal(t, "timeout", formatError(err))

	// Test case: err is nil
	assert.Empty(t, formatError(nil))

	// Test case: err is red.Nil
	assert.Empty(t, formatError(red.Nil))

	// Test case: err is io.EOF
	assert.Equal(t, "eof", formatError(io.EOF))

	// Test case: err is context.DeadlineExceeded
	assert.Equal(t, "context deadline", formatError(context.DeadlineExceeded))

	// Test case: err is breaker.ErrServiceUnavailable
	assert.Equal(t, "breaker open", formatError(breaker.ErrServiceUnavailable))

	// Test case: err is unknown
	assert.Equal(t, "unexpected error", formatError(errors.New("some error")))
}

type mockOpError struct{}

func (mockOpError) Error() string {
	return "mock error"
}

func (mockOpError) Timeout() bool {
	return true
}
