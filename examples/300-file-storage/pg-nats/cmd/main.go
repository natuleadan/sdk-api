package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/natuleadan/300-file-storage-pg-nats/models"

	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server"
)

func main() {
	mode := flag.String("mode", "entry", "run mode: entry (HTTP) or exit (workers)")
	flag.Parse()

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	svc, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	hooks := &models.ProductHooks{}
	runtime.MustRegister(svc, "Product", "pg-main", "products", hooks)

	var store server.StorageBackend

	svc.WithExit("onMediaUploaded", func(ctx context.Context, msg []byte) ([]byte, error) {
		var evt struct {
			Key         string `json:"key"`
			Spool       string `json:"spool"`
			Size        int64  `json:"size"`
			ContentType string `json:"contentType"`
		}
		if err := json.Unmarshal(msg, &evt); err != nil || evt.Spool == "" {
			log.Printf("Media event received: %s", string(msg))
			return []byte(`{"processed":true}`), nil
		}
		if store == nil {
			store = svc.Storage("/files/upload-async")
			if store == nil {
				return nil, fmt.Errorf("storage not configured")
			}
		}
		f, err := os.Open(evt.Spool)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		defer os.Remove(evt.Spool)
		if err := store.Upload(ctx, "uploads/"+evt.Key, f, evt.Size, evt.ContentType); err != nil {
			return nil, err
		}
		return []byte(`{"processed":true}`), nil
	})

	svc.WithExit("onMediaDeleted", func(ctx context.Context, msg []byte) ([]byte, error) {
		log.Printf("Media deleted event received: %s", string(msg))
		return []byte(`{"processed":true}`), nil
	})

	svc.WithRest("onFileUpload", func(c *runtime.RestCtx) error {
		if store == nil {
			store = svc.Storage("/files/upload")
			if store == nil {
				return c.Status(500).JSON(map[string]any{"error": "storage not configured"})
			}
			hooks.Store = store
		}
		key := fmt.Sprintf("%d", time.Now().UnixNano())
		objKey := fmt.Sprintf("uploads/%s", key)
		var err error
		if us, ok := store.(server.UploadStreamer); ok {
			err = us.UploadStream(c.Context(), objKey, c.Stream(), c.Get("Content-Type"))
		} else {
			body := c.Body()
			err = store.Upload(c.Context(), objKey, bytes.NewReader(body), int64(len(body)), c.Get("Content-Type"))
		}
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		resp := models.UploadResponse{Key: key, Size: contentLength(c)}
		if p, ok := store.(server.Presigner); ok {
			ttl := presignTTL(store)
			if url, err := p.PresignURL(c.Context(), objKey, ttl); err == nil {
				resp.PresignURL = url
			}
		}
		return c.JSON(resp)
	})

	svc.WithRest("onFileUploadAsync", func(c *runtime.RestCtx) error {
		if store == nil {
			store = svc.Storage("/files/upload-async")
			if store == nil {
				return c.Status(500).JSON(map[string]any{"error": "storage not configured"})
			}
		}
		var scfg server.SpoolConfig
		if p, ok := store.(interface{ SpoolConfig() server.SpoolConfig }); ok {
			scfg = p.SpoolConfig()
		}
		spool, err := server.SpoolToFile(c.Stream(), scfg)
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		key := fmt.Sprintf("%d", time.Now().UnixNano())
		evt := map[string]any{
			"key":         key,
			"spool":       spool.Path,
			"size":        spool.Size,
			"contentType": c.Get("Content-Type"),
		}
		if broker := svc.Stream("primary"); broker != nil {
			if err := broker.PublishJSON(c.Context(), "media.created", evt); err != nil {
				os.Remove(spool.Path)
				return c.Status(500).JSON(map[string]any{"error": err.Error()})
			}
		} else {
			os.Remove(spool.Path)
			return c.Status(500).JSON(map[string]any{"error": "stream not configured"})
		}
		return c.Status(202).JSON(map[string]any{"key": key, "status": "pending"})
	})

	svc.WithRest("onDownloadCached", func(c *runtime.RestCtx) error {
		if store == nil {
			store = svc.Storage("/files/upload")
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

	svc.WithRest("onGetProductWithMedia", func(c *runtime.RestCtx) error {
		if store == nil {
			store = svc.Storage("/files/upload")
		}
		var presigner server.Presigner
		if p, ok := store.(server.Presigner); ok {
			presigner = p
		}
		table := runtime.GetTable[models.Product](svc, "Product")
		if table == nil {
			return c.Status(500).JSON(map[string]any{"error": "product table not available"})
		}
		product, err := table.Get(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		pub, _ := models.TransformToPublic(product, presigner)
		return c.JSON(pub)
	})

	switch *mode {
	case "entry":
		log.Printf("starting file-pg-nats entry on :%d", 23304)
	case "exit":
		log.Printf("starting file-pg-nats exit workers")
	}

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func contentLength(c *runtime.RestCtx) int {
	if n, err := strconv.Atoi(c.Get("Content-Length")); err == nil {
		return n
	}
	return 0
}

func presignTTL(store any) time.Duration {
	if p, ok := store.(interface{ PresignTTL() time.Duration }); ok {
		return p.PresignTTL()
	}
	return 5 * time.Minute
}
