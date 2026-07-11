package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/natuleadan/sdk-api/runtime"
)

//go:embed service.yaml
var configYAML []byte

func main() {
	svc, err := runtime.NewFromYAML(configYAML)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svc.WithRest("onUpload", func(c *runtime.RestCtx) error {
		key := c.Params("key")
		body := c.Body()
		path := filepath.Join("/data", key)
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		if err := os.WriteFile(path, body, 0640); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.JSON(map[string]any{"uploaded": key, "size": len(body)})
	})

	svc.WithRest("onDownload", func(c *runtime.RestCtx) error {
		key := c.Params("key")
		path := filepath.Join("/data", key)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return c.Status(404).JSON(map[string]any{"error": "not found"})
			}
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, key))
		return c.Status(200).SendString(string(data))
	})

	svc.WithRest("onList", func(c *runtime.RestCtx) error {
		entries, err := os.ReadDir("/data")
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		files := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() {
				files = append(files, e.Name())
			}
		}
		return c.JSON(map[string]any{"files": files})
	})

	svc.WithRest("onDelete", func(c *runtime.RestCtx) error {
		key := c.Params("key")
		if err := os.Remove(filepath.Join("/data", key)); err != nil {
			if os.IsNotExist(err) {
				return c.Status(404).JSON(map[string]any{"error": "not found"})
			}
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.JSON(map[string]any{"deleted": key})
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
