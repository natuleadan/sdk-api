package filex

import (
	"os"
	"testing"

	"github.com/natuleadan/sdk-api/infra/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitLineChunks(t *testing.T) {
	const text = `first line
second line
third line
fourth line
fifth line
sixth line
seventh line
`
	fp, err := fs.TempFileWithText(text)
	require.NoError(t, err)
	defer func() {
		fp.Close()
		os.Remove(fp.Name())
	}()

	offsets, err := SplitLineChunks(fp.Name(), 3)
	require.NoError(t, err)
	body := make([]byte, 512)
	for _, offset := range offsets {
		reader := NewRangeReader(fp, offset.Start, offset.Stop)
		n, err := reader.Read(body)
		require.NoError(t, err)
		assert.Equal(t, uint8('\n'), body[n-1])
	}
}

func TestSplitLineChunksNoFile(t *testing.T) {
	_, err := SplitLineChunks("nosuchfile", 2)
	require.Error(t, err)
}

func TestSplitLineChunksFull(t *testing.T) {
	const text = `first line
second line
third line
fourth line
fifth line
sixth line
`
	fp, err := fs.TempFileWithText(text)
	require.NoError(t, err)
	defer func() {
		fp.Close()
		os.Remove(fp.Name())
	}()

	offsets, err := SplitLineChunks(fp.Name(), 1)
	require.NoError(t, err)
	body := make([]byte, 512)
	for _, offset := range offsets {
		reader := NewRangeReader(fp, offset.Start, offset.Stop)
		n, err := reader.Read(body)
		require.NoError(t, err)
		assert.Equal(t, []byte(text), body[:n])
	}
}
