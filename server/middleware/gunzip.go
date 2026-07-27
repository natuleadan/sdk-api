package middleware

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"github.com/gofiber/fiber/v3"
)

const gzipMagic = "\x1f\x8b\x08"

func Gunzip() fiber.Handler {
	return func(c fiber.Ctx) error {
		body := getRequestBody(c)
		if len(body) < 3 || string(body[:3]) != gzipMagic {
			return c.Next()
		}

		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err == nil {
			defer func() {
				if err := reader.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "gunzip: reader close error: %v\n", err)
				}
			}()
			buf := getBuffer()
			defer putBuffer(buf)
			if _, err := io.CopyN(buf, reader, 100<<20); err == nil || err == io.EOF {
				setRequestBody(c, buf.Bytes())
			} else {
				fmt.Fprintf(os.Stderr, "gunzip: decompress error: %v\n", err)
			}
		}

		return c.Next()
	}
}
