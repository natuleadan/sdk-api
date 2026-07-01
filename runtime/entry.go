package runtime

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/server"
	"github.com/natuleadan/sdk-api/server/middleware"
)

// CRUDProvider abstracts typed Table[T] operations for the router.
type CRUDProvider interface {
	List(ctx *fiber.Ctx, params ListParams) error
	Get(ctx *fiber.Ctx, id string) error
	Create(ctx *fiber.Ctx, body []byte) error
	Update(ctx *fiber.Ctx, id string, body []byte) error
	Delete(ctx *fiber.Ctx, id string) error
}

// ListParams holds query parameters for list endpoints.
type ListParams struct {
	Page    int
	Size    int
	Sort    string
	Filters map[string]string
}

// EntryHandlers holds all named handler functions registered by the user.
type EntryHandlers struct {
	Rest      map[string]func(*fiber.Ctx) error
	WS        map[string]WSHandler
	SSE       map[string]SSEHandler
	CRUD      map[string]CRUDProvider
	Storage   map[string]server.StorageBackend
	Async     map[string]AsyncHandler
	Transform map[string]any // handler name → EntryHooks[T] (untyped due to generics)
}

// RegisterEntries iterates over cfg.Entry and registers all HTTP routes on app.
// brokers is optional — used for auto-publishing on nats_publish targets.
// models is optional — used for GraphQL schema generation.
func RegisterEntries(app *fiber.App, cfg *ServiceConfig, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo) error {
	for i, entry := range cfg.Entry {
		if err := registerOneEntry(app, &entry, handlers, prefix, brokers, models); err != nil {
			return fmt.Errorf("entry[%d] %s %s: %w", i, entry.Type, entry.Path, err)
		}
	}
	return nil
}

func registerOneEntry(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo) error {
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
