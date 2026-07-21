package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	svc, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	var store server.StorageBackend

	svc.WithRest("onUpload", func(c *runtime.RestCtx) error {
		if store == nil {
			store = svc.Storage("/files/upload/:key")
			if store == nil {
				return c.Status(500).JSON(map[string]any{"error": "storage not configured"})
			}
		}
		key := c.Params("key")
		body := c.Body()
		if err := store.Upload(c.Context(), fmt.Sprintf("uploads/%s", key), bytes.NewReader(body), int64(len(body)), c.Get("Content-Type")); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.JSON(map[string]any{"uploaded": key, "size": len(body)})
	})

	svc.WithRest("onDownloadProxy", func(c *runtime.RestCtx) error {
		if store == nil {
			store = svc.Storage("/files/upload/:key")
			if store == nil {
				return c.Status(500).JSON(map[string]any{"error": "storage not configured"})
			}
		}
		key := c.Params("key")
		reader, err := store.Download(c.Context(), fmt.Sprintf("uploads/%s", key))
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		if len(data) == 0 {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, key))
		return c.Status(200).SendString(string(data))
	})

	svc.WithRest("onDownloadPresign", func(c *runtime.RestCtx) error {
		if store == nil {
			store = svc.Storage("/files/upload/:key")
			if store == nil {
				return c.Status(500).JSON(map[string]any{"error": "storage not configured"})
			}
		}
		key := c.Params("key")
		presigner, ok := store.(server.Presigner)
		if !ok {
			return c.Status(500).JSON(map[string]any{"error": "storage does not support presigned URLs"})
		}
		ttl := presignTTL(store)
		url, err := presigner.PresignURL(c.Context(), fmt.Sprintf("uploads/%s", key), ttl)
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.Redirect(url, 302)
	})

	svc.WithRest("onSignOnly", func(c *runtime.RestCtx) error {
		key := c.Params("key")
		backend := svc.Storage("/files/upload/:key")
		if backend == nil {
			return c.Status(500).JSON(map[string]any{"error": "storage not configured"})
		}
		presigner, ok := backend.(server.Presigner)
		if !ok {
			return c.Status(500).JSON(map[string]any{"error": "presigner not available"})
		}
		ttl := presignTTL(backend)
		url, err := presigner.PresignURL(c.Context(), fmt.Sprintf("uploads/%s", key), ttl)
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.JSON(map[string]any{
			"url":     url,
			"key":     key,
			"expires": ttl.String(),
		})
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func presignTTL(store any) time.Duration {
	if p, ok := store.(interface{ PresignTTL() time.Duration }); ok {
		return p.PresignTTL()
	}
	return 5 * time.Minute
}
