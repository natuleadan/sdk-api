package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/natuleadan/300-file-storage-pg-nats/models"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server"
)

//go:embed service.yaml
var configYAML []byte

func main() {
	mode := flag.String("mode", "entry", "run mode: entry (HTTP) or exit (workers)")
	flag.Parse()

	svc, err := runtime.NewFromYAML(configYAML)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	hooks := &models.ProductHooks{}
	runtime.MustRegister(svc, "Product", "pg-main", "products", hooks)

	var store server.StorageBackend

	svc.WithExit("onMediaUploaded", func(ctx context.Context, msg []byte) ([]byte, error) {
		log.Printf("Media uploaded event received: %s", string(msg))
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
		key := fmt.Sprintf("%d-%d", time.Now().UnixNano(), len(c.Body()))
		body := c.Body()
		objKey := fmt.Sprintf("uploads/%s", key)
		if err := store.Upload(c.Context(), objKey, bytes.NewReader(body), int64(len(body)), c.Get("Content-Type")); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		resp := models.UploadResponse{Key: key, Size: len(body)}
		if p, ok := store.(server.Presigner); ok {
			ttl := presignTTL(store)
			if url, err := p.PresignURL(c.Context(), objKey, ttl); err == nil {
				resp.PresignURL = url
			}
		}
		return c.JSON(resp)
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

func presignTTL(store any) time.Duration {
	if p, ok := store.(interface{ PresignTTL() time.Duration }); ok {
		return p.PresignTTL()
	}
	return 5 * time.Minute
}
