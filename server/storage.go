package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// StorageBackend abstracts file storage operations.
type StorageBackend interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// ---- S3 ----

type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
}

type S3Storage struct {
	client *minio.Client
	bucket string
}

func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Region: cfg.Region,
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: %w", err)
	}

	// Verify bucket exists
	exists, err := client.BucketExists(context.Background(), cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("minio bucket check: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("bucket %q does not exist", cfg.Bucket)
	}

	return &S3Storage{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Storage) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("s3 upload %s: %w", key, err)
	}
	return nil
}

func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("s3 download %s: %w", key, err)
	}
	return obj, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// ---- Local ----

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) (*LocalStorage, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("local storage path: %w", err)
	}
	if err := os.MkdirAll(abs, 0750); err != nil {
		return nil, fmt.Errorf("local storage mkdir: %w", err)
	}
	return &LocalStorage{root: abs}, nil
}

func (l *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader, _ int64, _ string) error {
	// Sanitize key to prevent path traversal
	key = sanitizeKey(key)
	fullPath := filepath.Join(l.root, key)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("local upload mkdir: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("local upload create: %w", err)
	}
	defer func() { if err := f.Close(); err != nil { fmt.Fprintf(os.Stderr, "storage: close error: %v\n", err) } }()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("local upload copy: %w", err)
	}
	return nil
}

func (l *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	key = sanitizeKey(key)
	fullPath := filepath.Join(l.root, key)
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", key)
		}
		return nil, fmt.Errorf("local download: %w", err)
	}
	return f, nil
}

func (l *LocalStorage) Delete(ctx context.Context, key string) error {
	key = sanitizeKey(key)
	fullPath := filepath.Join(l.root, key)
	return os.Remove(fullPath)
}

// sanitizeKey removes path traversal components.
func sanitizeKey(key string) string {
	key = filepath.Clean(key)
	for strings.HasPrefix(key, "../") || strings.HasPrefix(key, "/") {
		key = strings.TrimPrefix(key, "../")
		key = strings.TrimPrefix(key, "/")
	}
	if key == "." || key == ".." {
		return ""
	}
	return key
}
