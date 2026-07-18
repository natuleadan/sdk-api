package logx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLessWriter(t *testing.T) {
	var builder strings.Builder
	w := newLessWriter(&builder, 500)
	for range 100 {
		_, err := w.Write([]byte("hello"))
		require.NoError(t, err)
	}

	assert.Equal(t, "hello", builder.String())
}
