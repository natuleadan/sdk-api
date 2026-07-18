package logx

import (
	"fmt"
	"io"
	"os"
)

type lessWriter struct {
	*limitedExecutor
	writer io.Writer
}

func newLessWriter(writer io.Writer, milliseconds int) *lessWriter {
	return &lessWriter{
		limitedExecutor: newLimitedExecutor(milliseconds),
		writer:          writer,
	}
}

func (w *lessWriter) Write(p []byte) (n int, err error) {
	w.logOrDiscard(func() {
		if _, err := w.writer.Write(p); err != nil {
			fmt.Fprintf(os.Stderr, "logx: lesswriter write error: %v\n", err)
		}
	})
	return len(p), nil
}
