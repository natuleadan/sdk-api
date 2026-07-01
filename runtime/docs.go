package runtime

import (
	scalargo "github.com/bdpiprava/scalar-go"
	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/db"
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

	specPath := oai.SpecPath
	if specPath == "" {
		specPath = "/openapi.json"
	}
	app.Get(specPath, func(c *fiber.Ctx) error {
		c.Set("Content-Type", "application/json")
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
	app.Get(docsPath, func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(scalarHTML)
	})

	logx.Infof("docs: %s and %s", specPath, docsPath)
}
