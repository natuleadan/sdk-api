package iox

import (
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanner(t *testing.T) {
	const val = `1
2
3
4`
	reader := strings.NewReader(val)
	scanner := NewTextLineScanner(reader)
	var lines []string
	for scanner.Scan() {
		line, err := scanner.Line()
		require.NoError(t, err)
		lines = append(lines, line)
	}
	assert.Equal(t, []string{"1", "2", "3", "4"}, lines)
}

func TestBadScanner(t *testing.T) {
	scanner := NewTextLineScanner(iotest.ErrReader(iotest.ErrTimeout))
	assert.False(t, scanner.Scan())
	_, err := scanner.Line()
	assert.ErrorIs(t, err, iotest.ErrTimeout)
}
