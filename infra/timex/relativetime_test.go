package timex

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRelativeTime(t *testing.T) {
	time.Sleep(time.Millisecond)
	now := Now()
	assert.Positive(t, now)
	time.Sleep(time.Millisecond)
	assert.Positive(t, Since(now))
}

func BenchmarkTimeSince(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = time.Since(time.Now())
	}
}

func BenchmarkTimexSince(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = Since(Now())
	}
}
