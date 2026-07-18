package logx

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetCaller(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	assert.Contains(t, getCaller(1), filepath.Base(file))
	assert.Empty(t, getCaller(1<<10))
}

func TestGetTimestamp(t *testing.T) {
	ts := getTimestamp()
	tm, err := time.Parse(timeFormat, ts)
	assert.NoError(t, err)
	assert.Less(t, time.Since(tm), time.Minute)
}

func TestPrettyCaller(t *testing.T) {
	tests := []struct {
		name string
		file string
		line int
		want string
	}{
		{
			name: "regular",
			file: "logx_test.go",
			line: 123,
			want: "logx_test.go:123",
		},
		{
			name: "relative",
			file: "adhoc/logx_test.go",
			line: 123,
			want: "adhoc/logx_test.go:123",
		},
		{
			name: "long path",
			file: "github.com/natuleadan/sdk-api/infra/logx/util_test.go",
			line: 12,
			want: "logx/util_test.go:12",
		},
		{
			name: "local path",
			file: "/home/user/project/logx/util_test.go",
			line: 1234,
			want: "logx/util_test.go:1234",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, prettyCaller(test.file, test.line))
		})
	}
}

func BenchmarkGetCaller(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		getCaller(1)
	}
}
