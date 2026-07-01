package codec

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

const unzipLimit = 100 * 1024 * 1024 // 100MB

// Gzip compresses bs.
func Gzip(bs []byte) []byte {
	var b bytes.Buffer

	w := gzip.NewWriter(&b)
	if _, err := w.Write(bs); err != nil {
		fmt.Printf("gzip write: %v\n", err)
	}
	if err := w.Close(); err != nil {
		fmt.Printf("gzip close: %v\n", err)
	}

	return b.Bytes()
}

// Gunzip uncompresses bs.
func Gunzip(bs []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewBuffer(bs))
	if err != nil {
		return nil, err
	}
	defer func() { if err := r.Close(); err != nil { fmt.Printf("close error: %v\n", err) } }()

	var c bytes.Buffer
	if _, err = io.Copy(&c, io.LimitReader(r, unzipLimit)); err != nil {
		return nil, err
	}

	return c.Bytes(), nil
}
