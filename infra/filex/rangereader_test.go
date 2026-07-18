package filex

import (
	"os"
	"testing"

	"github.com/natuleadan/sdk-api/infra/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRangeReader(t *testing.T) {
	const text = `hello
world`
	file, err := fs.TempFileWithText(text)
	require.NoError(t, err)
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()

	reader := NewRangeReader(file, 5, 8)
	buf := make([]byte, 10)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.InDelta(t, 3, n, 0.01)
	assert.Equal(t, `
wo`, string(buf[:n]))
}

func TestRangeReader_OutOfRange(t *testing.T) {
	const text = `hello
world`
	file, err := fs.TempFileWithText(text)
	require.NoError(t, err)
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()

	reader := NewRangeReader(file, 50, 8)
	buf := make([]byte, 10)
	_, err = reader.Read(buf)
	require.Error(t, err)
}
