package runtime

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"sync"

	scalargo "github.com/bdpiprava/scalar-go"
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/hash"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// registerDocs registers /openapi.json and /docs (Scalar UI) endpoints
// if the server.openapi.enabled config is true.
func registerDocs(app *fiber.App, cfg *ServiceConfig, models map[string]*db.TableInfo) {
	oai := cfg.Server.OpenAPI
	if oai == nil || !oai.Enabled {
		return
	}

	spec, err := BuildOpenAPI(cfg, models)
	if err != nil {
		logx.Errorf("openapi build: %v", err)
		return
	}

	jsonData, err := spec.MarshalJSON()
	if err != nil {
		logx.Errorf("openapi marshal: %v", err)
		return
	}

	var (
		compressedSpec []byte
		compressOnce   sync.Once
	)
	compressOnce.Do(func() {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write(jsonData); err != nil {
			logx.Errorf("gzip write: %v", err)
		}
		if err := gw.Close(); err != nil {
			logx.Errorf("gzip close: %v", err)
		}
		compressedSpec = buf.Bytes()
	})

	specPath := oai.SpecPath
	if specPath == "" {
		specPath = "/openapi.json"
	}
	app.Get(specPath, func(c fiber.Ctx) error {
		c.Set("Content-Type", "application/json")
		if strings.Contains(c.Get("Accept-Encoding"), "gzip") && len(compressedSpec) > 0 {
			c.Set("Content-Encoding", "gzip")
			return c.Send(compressedSpec)
		}
		etag := fmt.Sprintf(`"%x"`, hash.Md5(jsonData))
		c.Set("ETag", etag)
		c.Set("Cache-Control", "public, max-age=3600")
		if c.Get("If-None-Match") == etag {
			c.Status(fiber.StatusNotModified)
			return nil
		}
		return c.Send(jsonData)
	})

	opts := []scalargo.Option{
		scalargo.WithSpecBytes(jsonData),
		scalargo.WithDefaultFonts(),
	}
	if oai.DarkMode {
		opts = append(opts, scalargo.WithDarkMode())
	}

	scalarHTML, err := scalargo.NewV2(opts...)
	if err != nil {
		logx.Errorf("scalar render: %v", err)
		return
	}

	docsPath := oai.DocsPath
	if docsPath == "" {
		docsPath = "/docs"
	}
	app.Get(docsPath, func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(scalarHTML)
	})

	logx.Infof("docs: %s and %s", specPath, docsPath)
}
