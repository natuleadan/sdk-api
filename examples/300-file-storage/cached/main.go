package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"log"

	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server"
)

//go:embed service.yaml
var configYAML []byte

func main() {
	svc, err := runtime.NewFromYAML(configYAML)
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
		objKey := fmt.Sprintf("uploads/%s", key)
		if err := store.Upload(c.Context(), objKey, bytes.NewReader(body), int64(len(body)), c.Get("Content-Type")); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.JSON(map[string]any{"uploaded": key, "size": len(body)})
	})

	svc.WithRest("onDownloadCached", func(c *runtime.RestCtx) error {
		if store == nil {
			store = svc.Storage("/files/upload/:key")
			if store == nil {
				return c.Status(500).JSON(map[string]any{"error": "storage not configured"})
			}
		}
		key := c.Params("key")
		objKey := fmt.Sprintf("uploads/%s", key)
		reader, err := store.Download(c.Context(), objKey)
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil || len(data) == 0 {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		return c.Status(200).SendString(string(data))
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
