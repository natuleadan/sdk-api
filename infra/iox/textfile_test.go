package iox

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountLines(t *testing.T) {
	const val = `1
2
3
4`
	file, err := os.CreateTemp(os.TempDir(), "test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())

	file.WriteString(val)
	file.Close()
	lines, err := CountLines(file.Name())
	require.NoError(t, err)
	assert.InDelta(t, 4, lines, 0.01)
}

func TestCountLinesError(t *testing.T) {
	_, err := CountLines("not-exist")
	require.Error(t, err)
}
