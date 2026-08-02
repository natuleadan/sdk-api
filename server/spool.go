package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/natuleadan/sdk-api/infra/logx"
)

// SpoolConfig drives streaming uploads: ingest the request body to memory
// (bounded by MemoryLimit) and then to local disk, before uploading the
// assembled file to the backend (S3) with multipart. All fields are
// YAML-driven via storage.spool.
//
// Mode selects where the ingest buffer lives:
//   - "auto" (default): memory up to MemoryLimit, then spill to disk
//   - "memory": buffer entirely in RAM (small payloads)
//   - "disk": stream directly to a temp file, no RAM accumulation
type SpoolConfig struct {
	Mode        string
	MemoryLimit int64
	Dir         string
	PartSize    int64
	Concurrency int
	Async       bool
}

// UploadStreamer is an optional extension of StorageBackend for streaming
// uploads that do not require the whole payload in memory.
type UploadStreamer interface {
	UploadStream(ctx context.Context, key string, stream io.Reader, contentType string) error
}

// SpooledFile is a streamed-to-disk payload ready for upload. The caller
// owns the file and must remove it after use (os.Remove on Path).
type SpooledFile struct {
	Path string
	Size int64
}

// SpoolToFile ingests the stream keeping at most cfg.MemoryLimit bytes in
// RAM; beyond that it spills to a temp file in cfg.Dir. The temp file is NOT
// removed here — the caller decides (immediate upload or async handoff).
func SpoolToFile(stream io.Reader, cfg SpoolConfig) (*SpooledFile, error) {
	limit := cfg.MemoryLimit
	if limit <= 0 {
		limit = 4 << 20
	}
	dir := cfg.Dir
	if dir == "" {
		dir = os.TempDir()
	}
	mode := cfg.Mode
	if mode == "" {
		mode = "auto"
	}

	sp := &spooler{buf: make([]byte, 64*1024), limit: limit, dir: dir, mode: mode}
	if err := sp.ingest(stream); err != nil {
		sp.cleanup()
		return nil, err
	}
	return sp.finish()
}

// spooler accumulates the stream in memory up to limit, then spills to disk.
type spooler struct {
	mem   bytes.Buffer
	file  *os.File
	path  string
	size  int64
	buf   []byte
	limit int64
	dir   string
	mode  string
}

func (sp *spooler) ingest(stream io.Reader) error {
	for {
		n, err := stream.Read(sp.buf)
		if n > 0 {
			if werr := sp.writeChunk(sp.buf[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (sp *spooler) writeChunk(chunk []byte) error {
	useDisk := sp.mode == "disk" || (sp.mode != "memory" && sp.file == nil && int64(sp.mem.Len())+int64(len(chunk)) > sp.limit)
	if useDisk && sp.file == nil {
		if err := sp.spill(); err != nil {
			return err
		}
	}
	if sp.file != nil {
		if _, err := sp.file.Write(chunk); err != nil {
			return err
		}
	} else {
		_, _ = sp.mem.Write(chunk)
	}
	sp.size += int64(len(chunk))
	return nil
}

func (sp *spooler) spill() error {
	f, err := os.CreateTemp(sp.dir, "spool-*.tmp")
	if err != nil {
		return err
	}
	sp.file = f
	sp.path = f.Name()
	if _, err := f.Write(sp.mem.Bytes()); err != nil {
		return err
	}
	sp.mem.Reset()
	return nil
}

func (sp *spooler) finish() (*SpooledFile, error) {
	if sp.file == nil {
		if err := sp.spill(); err != nil {
			return nil, err
		}
	}
	if err := sp.file.Close(); err != nil {
		return nil, err
	}
	return &SpooledFile{Path: sp.path, Size: sp.size}, nil
}

func (sp *spooler) cleanup() {
	if sp.file != nil {
		_ = sp.file.Close()
	}
	if sp.path != "" {
		_ = os.Remove(sp.path)
	}
}

// spooledStorage ingests the incoming stream to memory (up to MemoryLimit)
// and then to a temp file on local disk, and uploads the assembled file to
// the wrapped backend afterwards. The only ingest bound is local disk speed.
type spooledStorage struct {
	StorageBackend
	cfg SpoolConfig
}

// NewSpooledStorage wraps a backend with spool-to-disk streaming uploads.
func NewSpooledStorage(backend StorageBackend, cfg SpoolConfig) StorageBackend {
	return &spooledStorage{StorageBackend: backend, cfg: cfg}
}

// UploadStream spools the stream first, then uploads with a known size so
// the backend can use multipart (part size and concurrency YAML-driven).
func (s *spooledStorage) UploadStream(ctx context.Context, key string, stream io.Reader, contentType string) error {
	spool, err := SpoolToFile(stream, s.cfg)
	if err != nil {
		return fmt.Errorf("spool: %w", err)
	}
	defer func() {
		if rerr := os.Remove(spool.Path); rerr != nil {
			logx.Errorf("spool remove %s: %v", spool.Path, rerr)
		}
	}()
	f, err := os.Open(spool.Path)
	if err != nil {
		return fmt.Errorf("spool open: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			logx.Errorf("spool close %s: %v", spool.Path, cerr)
		}
	}()
	return s.StorageBackend.Upload(ctx, key, f, spool.Size, contentType)
}

// Upload delegates to the wrapped backend for already-materialized readers.
func (s *spooledStorage) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	return s.StorageBackend.Upload(ctx, key, reader, size, contentType)
}

// PresignTTL propagates the wrapped backend's presign TTL.
func (s *spooledStorage) PresignTTL() time.Duration {
	if p, ok := s.StorageBackend.(interface{ PresignTTL() time.Duration }); ok {
		return p.PresignTTL()
	}
	return 5 * time.Minute
}

// PresignURL propagates the wrapped backend's presigned URL generation.
func (s *spooledStorage) PresignURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if p, ok := s.StorageBackend.(Presigner); ok {
		return p.PresignURL(ctx, key, ttl)
	}
	return "", fmt.Errorf("underlying storage does not support presigned URLs")
}
