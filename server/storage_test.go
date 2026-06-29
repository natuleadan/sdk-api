package server

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorage(t *testing.T) {
	dir := t.TempDir()
	storage, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	ctx := context.Background()
	key := "test/file.txt"
	content := "hello world"
	reader := stringsToSeeker(content)

	// Upload
	err = storage.Upload(ctx, key, reader, int64(len(content)), "text/plain")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Verify file exists on disk
	data, err := os.ReadFile(filepath.Join(dir, key))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("content = %q, want %q", data, content)
	}

	// Download
	rc, err := storage.Download(ctx, key)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()

	downloaded, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(downloaded) != content {
		t.Errorf("downloaded = %q, want %q", downloaded, content)
	}

	// Delete
	err = storage.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Should not exist
	_, err = storage.Download(ctx, key)
	if err == nil {
		t.Error("expected error for deleted file")
	}

	// Download non-existent
	_, err = storage.Download(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLocalStorage_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	storage, _ := NewLocalStorage(dir)

	ctx := context.Background()
	content := "traversal attempt"

	// Try to write outside root
	err := storage.Upload(ctx, "../../../etc/passwd", stringsToSeeker(content), int64(len(content)), "text/plain")
	if err != nil {
		t.Fatalf("Upload (should sanitize): %v", err)
	}

	// File should be inside root, not in /etc
	safeKey := filepath.Join(dir, "etc", "passwd")
	data, err := os.ReadFile(safeKey)
	if err != nil {
		t.Fatalf("file should exist at safe path %q: %v", safeKey, err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: %q vs %q", string(data), content)
	}
}

func TestLocalStorage_Subdirectories(t *testing.T) {
	dir := t.TempDir()
	storage, _ := NewLocalStorage(dir)

	ctx := context.Background()
	key := "deep/nested/folder/file.bin"
	content := "nested"

	err := storage.Upload(ctx, key, stringsToSeeker(content), int64(len(content)), "application/octet-stream")
	if err != nil {
		t.Fatalf("Upload nested: %v", err)
	}

	rc, err := storage.Download(ctx, key)
	if err != nil {
		t.Fatalf("Download nested: %v", err)
	}
	defer rc.Close()

	downloaded, _ := io.ReadAll(rc)
	if string(downloaded) != content {
		t.Errorf("nested content = %q", string(downloaded))
	}
}

func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"file.txt", "file.txt"},
		{"../../../etc/passwd", "etc/passwd"},
		{"/absolute/path", "absolute/path"},
		{"./relative", "relative"},
		{"a/b/c", "a/b/c"},
		{"..", ""},
		{".", ""},
	}
	for _, tt := range tests {
		got := sanitizeKey(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// Helper: convert string to io.Reader for testing
func stringsToSeeker(s string) io.ReadSeeker {
	return &strSeeker{s: s}
}

type strSeeker struct {
	s   string
	pos int64
}

func (r *strSeeker) Read(p []byte) (int, error) {
	if r.pos >= int64(len(r.s)) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.pos:])
	r.pos += int64(n)
	return n, nil
}

func (r *strSeeker) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.pos + offset
	case io.SeekEnd:
		abs = int64(len(r.s)) + offset
	}
	if abs < 0 {
		return 0, os.ErrInvalid
	}
	r.pos = abs
	return abs, nil
}
