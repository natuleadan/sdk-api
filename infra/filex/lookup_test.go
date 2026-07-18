package filex

import (
	"os"
	"testing"

	"github.com/natuleadan/sdk-api/infra/fs"
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	defer func() {
		fp.Close()
		os.Remove(fp.Name())
	}()

	offsets, err := SplitLineChunks(fp.Name(), 3)
	assert.NoError(t, err)
	body := make([]byte, 512)
	for _, offset := range offsets {
		reader := NewRangeReader(fp, offset.Start, offset.Stop)
		n, err := reader.Read(body)
		assert.NoError(t, err)
		assert.Equal(t, uint8('\n'), body[n-1])
	}
}

func TestSplitLineChunksNoFile(t *testing.T) {
	_, err := SplitLineChunks("nosuchfile", 2)
	assert.Error(t, err)
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
	assert.NoError(t, err)
	defer func() {
		fp.Close()
		os.Remove(fp.Name())
	}()

	offsets, err := SplitLineChunks(fp.Name(), 1)
	assert.NoError(t, err)
	body := make([]byte, 512)
	for _, offset := range offsets {
		reader := NewRangeReader(fp, offset.Start, offset.Stop)
		n, err := reader.Read(body)
		assert.NoError(t, err)
		assert.Equal(t, []byte(text), body[:n])
	}
}
