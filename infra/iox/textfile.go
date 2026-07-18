package iox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const bufSize = 32 * 1024

// CountLines returns the number of lines in the file.
func CountLines(file string) (int, error) {
	f, err := os.Open(filepath.Clean(file))
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "textfile: close error: %v\n", err)
		}
	}()

	var noEol bool
	buf := make([]byte, bufSize)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := f.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case errors.Is(err, io.EOF):
			if noEol {
				count++
			}
			return count, nil
		case err != nil:
			return count, err
		}

		noEol = checkNoEol(buf, c)
	}
}

func checkNoEol(buf []byte, c int) bool {
	if c <= 0 || c > len(buf) {
		return false
	}
	return !bytes.HasSuffix(buf[:c], []byte{'\n'})
}
