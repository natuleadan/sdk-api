package server

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSpoolToFile_MemoryOnly(t *testing.T) {
	dir := t.TempDir()
	data := []byte("small payload")
	spool, err := SpoolToFile(bytes.NewReader(data), SpoolConfig{Mode: "auto", Dir: dir, MemoryLimit: 1 << 20})
	if err != nil {
		t.Fatalf("spool: %v", err)
	}
	defer os.Remove(spool.Path)
	if spool.Size != int64(len(data)) {
		t.Errorf("size: got %d want %d", spool.Size, len(data))
	}
	got, err := os.ReadFile(spool.Path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("content mismatch")
	}
}

func TestSpoolToFile_SpillsToDisk(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), 1<<20+128)
	spool, err := SpoolToFile(bytes.NewReader(data), SpoolConfig{Mode: "auto", Dir: dir, MemoryLimit: 64 * 1024})
	if err != nil {
		t.Fatalf("spool: %v", err)
	}
	defer os.Remove(spool.Path)
	if spool.Size != int64(len(data)) {
		t.Errorf("size: got %d want %d", spool.Size, len(data))
	}
	if filepath.Base(spool.Path) == "" {
		t.Error("expected a temp file path")
	}
	got, err := os.ReadFile(spool.Path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("spilled content mismatch (len %d vs %d)", len(got), len(data))
	}
}

func TestSpoolToFile_StreamError(t *testing.T) {
	_, err := SpoolToFile(errReader{}, SpoolConfig{Dir: t.TempDir()})
	if err == nil {
		t.Fatal("expected error from broken stream")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestSpooledStorage_UploadStream(t *testing.T) {
	dir := t.TempDir()
	backend := &recordingStorage{}
	store := NewSpooledStorage(backend, SpoolConfig{Mode: "auto", Dir: dir, MemoryLimit: 128, PartSize: 1 << 20, Concurrency: 2})
	data := bytes.Repeat([]byte("y"), 512)
	if err := store.(UploadStreamer).UploadStream(t.Context(), "k", bytes.NewReader(data), "text/plain"); err != nil {
		t.Fatalf("upload stream: %v", err)
	}
	if backend.size != int64(len(data)) {
		t.Errorf("backend size: got %d want %d", backend.size, len(data))
	}
	if !bytes.Equal(backend.data, data) {
		t.Errorf("backend content mismatch")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("temp spool file not cleaned up: %d entries", len(entries))
	}
}

type recordingStorage struct {
	data []byte
	size int64
}

func (r *recordingStorage) Upload(_ context.Context, _ string, reader io.Reader, size int64, _ string) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	r.data = data
	r.size = size
	return nil
}

func (r *recordingStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(r.data)), nil
}

func (r *recordingStorage) Delete(_ context.Context, _ string) error { return nil }

func TestSpoolToFile_DiskMode(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("z"), 2048)
	spool, err := SpoolToFile(bytes.NewReader(data), SpoolConfig{Mode: "disk", Dir: dir, MemoryLimit: 1 << 30})
	if err != nil {
		t.Fatalf("spool: %v", err)
	}
	defer os.Remove(spool.Path)
	got, err := os.ReadFile(spool.Path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("disk-mode content mismatch")
	}
}

func TestSpoolToFile_MemoryMode(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("w"), 1<<20)
	spool, err := SpoolToFile(bytes.NewReader(data), SpoolConfig{Mode: "memory", Dir: dir, MemoryLimit: 1})
	if err != nil {
		t.Fatalf("spool: %v", err)
	}
	defer os.Remove(spool.Path)
	if spool.Size != int64(len(data)) {
		t.Errorf("size: got %d want %d", spool.Size, len(data))
	}
	got, err := os.ReadFile(spool.Path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("memory-mode content mismatch")
	}
}
