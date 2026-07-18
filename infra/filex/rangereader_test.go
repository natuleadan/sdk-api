package filex

import (
	"os"
	"testing"

	"github.com/natuleadan/sdk-api/infra/fs"
	"github.com/stretchr/testify/assert"
)

func TestRangeReader(t *testing.T) {
	const text = `hello
world`
	file, err := fs.TempFileWithText(text)
	assert.NoError(t, err)
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()

	reader := NewRangeReader(file, 5, 8)
	buf := make([]byte, 10)
	n, err := reader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, `
wo`, string(buf[:n]))
}

func TestRangeReader_OutOfRange(t *testing.T) {
	const text = `hello
world`
	file, err := fs.TempFileWithText(text)
	assert.NoError(t, err)
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()

	reader := NewRangeReader(file, 50, 8)
	buf := make([]byte, 10)
	_, err = reader.Read(buf)
	assert.Error(t, err)
}
