package runtime

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/server"
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
	Transform map[string]any // handler name → EntryHooks[T] (untyped due to generics)
}

// RegisterEntries iterates over cfg.Entry and registers all HTTP routes on app.
// natsConns is optional — used for auto-publishing on nats_publish targets.
func RegisterEntries(app *fiber.App, cfg *ServiceConfig, handlers *EntryHandlers, prefix string, natsConns map[string]*events.Conn) error {
	for i, entry := range cfg.Entry {
		if err := registerOneEntry(app, &entry, handlers, prefix, natsConns); err != nil {
			return fmt.Errorf("entry[%d] %s %s: %w", i, entry.Type, entry.Path, err)
		}
	}
	return nil
}

func registerOneEntry(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, natsConns map[string]*events.Conn) error {
	switch entry.Type {
	case "crud":
		return registerCRUD(app, entry, handlers, prefix)
	case "rest":
		return registerREST(app, entry, handlers, prefix, natsConns)
	case "webhook":
		return registerREST(app, entry, handlers, prefix, natsConns)
	case "websocket":
		return registerWebSocket(app, entry, handlers, prefix)
	case "sse":
		return registerSSE(app, entry, handlers, prefix)
	case "file":
		return registerFile(app, entry, handlers, prefix)
	default:
		return fmt.Errorf("unknown entry type %q", entry.Type)
	}
}
