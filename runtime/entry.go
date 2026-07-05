package runtime

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/server"
	"github.com/natuleadan/sdk-api/server/auth/openfga"
	"github.com/natuleadan/sdk-api/server/auth/ory"
	"github.com/natuleadan/sdk-api/server/auth/zitadel"
	"github.com/natuleadan/sdk-api/server/middleware"
)

type CRUDProvider interface {
	List(ctx fiber.Ctx, params ListParams) error
	Get(ctx fiber.Ctx, id string) error
	Create(ctx fiber.Ctx, body []byte) error
	Update(ctx fiber.Ctx, id string, body []byte) error
	Delete(ctx fiber.Ctx, id string) error
}

type ListParams struct {
	Page    int
	Size    int
	Sort    string
	Filters map[string]string
}

type EntryHandlers struct {
	Rest      map[string]func(fiber.Ctx) error
	WS        map[string]WSHandler
	SSE       map[string]SSEHandler
	CRUD      map[string]CRUDProvider
	Storage   map[string]server.StorageBackend
	Async     map[string]AsyncHandler
	Transform map[string]any
}

func RegisterEntries(app *fiber.App, cfg *ServiceConfig, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client) error {
	driver := ""
	if cfg.Auth != nil {
		driver = cfg.Auth.Driver
	}
	for i, entry := range cfg.Entry {
		if entry.Auth {
			if err := validateEntryAuth(&entry, handlers); err != nil {
				return fmt.Errorf("entry[%d] %s:%s: %w", i, entry.Type, entry.Path, err)
			}
		}
		if err := registerOneEntry(app, &entry, handlers, prefix, brokers, models, jwtCfg, authValidator, fgaClient, oryClient, zitadelClient, driver); err != nil {
			return fmt.Errorf("entry[%d] %s %s: %w", i, entry.Type, entry.Path, err)
		}
	}
	return nil
}

func validateEntryAuth(entry *EntryDef, handlers *EntryHandlers) error {
	if len(entry.Roles) == 0 && len(entry.Permissions) == 0 {
		return nil
	}
	if entry.Handler != "" {
		if handlers.Rest != nil {
			if _, ok := handlers.Rest[entry.Handler]; ok {
				return nil
			}
		}
		return fmt.Errorf("handler %q not registered", entry.Handler)
	}
	if entry.Resource != "" && entry.Type == "crud" {
		if handlers.CRUD != nil {
			if _, ok := handlers.CRUD[entry.Resource]; ok {
				return nil
			}
		}
	}
	return nil
}

func registerOneEntry(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client, driver string) error {
	// Register auth middleware BEFORE route handler (Fiber requires app.Use before app.Get)
	switch driver {
	case "openfga-zitadel":
		if zitadelClient != nil {
			registerZitadelJWT(app, entry, prefix, jwtCfg, zitadelClient)
		} else {
			registerJWT(app, entry, prefix, jwtCfg)
		}
		registerOpenFGA(app, entry, prefix, fgaClient, entry.Roles, entry.Permissions)
	case "ory":
		registerJWT(app, entry, prefix, jwtCfg)
		registerOry(app, entry, prefix, oryClient, entry.Roles, entry.Permissions)
	case "manual":
		registerJWT(app, entry, prefix, jwtCfg)
		registerManualAuth(app, entry, prefix, entry.Roles, entry.Permissions, authValidator)
	default:
		registerJWT(app, entry, prefix, jwtCfg)
	}

	var err error
	switch entry.Type {
	case "crud":
		err = registerCRUD(app, entry, handlers, prefix, brokers)
	case "rest":
		err = registerREST(app, entry, handlers, prefix, brokers)
	case "webhook":
		err = registerREST(app, entry, handlers, prefix, brokers)
	case "websocket":
		err = registerWebSocket(app, entry, handlers, prefix)
	case "sse":
		err = registerSSE(app, entry, handlers, prefix)
	case "file":
		err = registerFile(app, entry, handlers, prefix, brokers)
	case "async":
		err = registerAsync(app, entry, handlers, prefix)
	case "graphql":
		err = registerGraphQL(app, entry, handlers, prefix, models)
	default:
		return fmt.Errorf("unknown entry type %q", entry.Type)
	}
	if err != nil {
		return err
	}
	registerValidationMiddleware(app, entry, prefix)
	registerEntryRateLimit(app, entry, prefix)
	return nil
}

func registerOpenFGA(app *fiber.App, entry *EntryDef, prefix string, fgaClient openfga.Checker, roles, permissions []string) {
	if !entry.Auth || fgaClient == nil {
		return
	}
	app.Use(prefix+entry.Path, middleware.OpenFGA(middleware.OpenFGAConfig{
		Client:      fgaClient,
		Roles:       roles,
		Permissions: permissions,
	}))
}

func registerOry(app *fiber.App, entry *EntryDef, prefix string, oryClient *ory.Client, roles, permissions []string) {
	if !entry.Auth || oryClient == nil {
		return
	}
	app.Use(prefix+entry.Path, middleware.Ory(middleware.OryConfig{
		Client:      oryClient,
		Roles:       roles,
		Permissions: permissions,
	}))
}

func registerJWT(app *fiber.App, entry *EntryDef, prefix string, jwtCfg *middleware.JWTConfig) {
	if !entry.Auth || jwtCfg == nil {
		return
	}
	app.Use(prefix+entry.Path, middleware.JWT(*jwtCfg))
}

func registerZitadelJWT(app *fiber.App, entry *EntryDef, prefix string, jwtCfg *middleware.JWTConfig, zClient *zitadel.Client) {
	if !entry.Auth || zClient == nil {
		return
	}
	app.Use(prefix+entry.Path, middleware.JWTWithZitadel(*jwtCfg, zClient))
}

func registerManualAuth(app *fiber.App, entry *EntryDef, prefix string, roles, permissions []string, validator func(context.Context, *middleware.AuthContext, []string, []string) error) {
	if !entry.Auth || validator == nil {
		return
	}
	app.Use(prefix+entry.Path, func(c fiber.Ctx) error {
		auth := middleware.GetAuth(c)
		if auth == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "auth context required",
			})
		}
		if err := validator(c.Context(), auth, roles, permissions); err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code":    403,
				"message": err.Error(),
			})
		}
		return c.Next()
	})
}

func registerValidationMiddleware(app *fiber.App, entry *EntryDef, prefix string) {
	if entry.ValidationModel == "" {
		return
	}
	if entry.Type != "crud" && entry.Type != "rest" && entry.Type != "webhook" {
		return
	}
	app.Use(prefix+entry.Path, middleware.ValidateInput(entry.ValidationModel))
}

func registerEntryRateLimit(app *fiber.App, entry *EntryDef, prefix string) {
	if entry.RateLimit == nil || entry.RateLimit.RequestsPerSecond <= 0 {
		return
	}
	rlCfg := middleware.RateLimitConfig{
		Global: &middleware.RateLimitEntry{
			RequestsPerSecond: entry.RateLimit.RequestsPerSecond,
			Burst:             entry.RateLimit.Burst,
		},
	}
	app.Use(prefix+entry.Path, middleware.RateLimit(rlCfg))
}

// fiber:context-methods migrated
