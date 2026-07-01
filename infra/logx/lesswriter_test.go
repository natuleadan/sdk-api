package logx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLessWriter(t *testing.T) {
	var builder strings.Builder
	w := newLessWriter(&builder, 500)
	for range 100 {
		_, err := w.Write([]byte("hello"))
		assert.Nil(t, err)
	}

	assert.Equal(t, "hello", builder.String())
}
