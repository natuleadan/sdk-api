package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
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
	Page       int
	Size       int
	Sort       string
	Filters    map[string]string
	Cursor     string // keyset pagination cursor
	Pagination string // "offset" | "keyset"
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

func RegisterEntries(app *fiber.App, cfg *ServiceConfig, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, apiKeyValidator func(ctx context.Context, key string) (*middleware.AuthContext, error), fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client) error {
	driver := ""
	if cfg.Auth != nil {
		driver = cfg.Auth.Driver
	}
	for i, entry := range cfg.Entry {
		if len(entry.AuthModes) > 0 {
			if err := validateEntryAuth(&entry, handlers); err != nil {
				return fmt.Errorf("entry[%d] %s:%s: %w", i, entry.Type, entry.Path, err)
			}
		}
		if err := registerOneEntry(app, &entry, handlers, prefix, brokers, models, jwtCfg, authValidator, apiKeyValidator, fgaClient, oryClient, zitadelClient, driver); err != nil {
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

func registerAuthMiddleware(entry *EntryDef, driver string, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, apiKeyValidator func(ctx context.Context, key string) (*middleware.AuthContext, error), fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client) []fiber.Handler {
	var mws []fiber.Handler
	hasAPIKey := hasAuth(entry, "apikey")
	hasJWT := hasAuth(entry, "jwt") && driver != "none" && driver != ""

	if hasAPIKey && hasJWT && jwtReadsHeader(entry, jwtCfg) {
		mws = append(mws, authRouter(entry))
	}

	if hasAPIKey {
		mws = append(mws, apiKeyMiddleware(entry, apiKeyValidator, fgaClient))
		mws = append(mws, apiKeyRoleMiddleware(entry, authValidator)...)
	}

	if !hasJWT {
		return mws
	}

	switch driver {
	case "openfga-zitadel":
		if zitadelClient != nil {
			mws = append(mws, zitadelJWTMiddleware(entry, jwtCfg, zitadelClient))
		} else {
			mws = append(mws, jwtMiddleware(entry, jwtCfg))
		}
		if fgaClient != nil {
			mws = append(mws, openfgaMiddleware(entry, fgaClient, entry.Roles, entry.Permissions))
		}
	case "ory":
		mws = append(mws, jwtMiddleware(entry, jwtCfg))
		if oryClient != nil {
			mws = append(mws, oryMiddleware(entry, oryClient, entry.Roles, entry.Permissions))
		}
	case "manual":
		mws = append(mws, jwtMiddleware(entry, jwtCfg))
		mws = append(mws, manualAuthMiddleware(entry, entry.Roles, entry.Permissions, authValidator))
	default:
		mws = append(mws, jwtMiddleware(entry, jwtCfg))
	}
	return mws
}

func jwtReadsHeader(entry *EntryDef, jwtCfg *middleware.JWTConfig) bool {
	lookup := entry.JWTFrom
	if lookup == "" {
		if jwtCfg == nil {
			return false
		}
		lookup = jwtCfg.TokenLookup
	}
	if lookup == "" {
		return true
	}
	return strings.HasPrefix(lookup, "header:")
}

func authRouter(entry *EntryDef) fiber.Handler {
	prefix := entry.APIPrefix
	return func(c fiber.Ctx) error {
		raw := c.Get("Authorization")
		if strings.HasPrefix(raw, "Bearer ") {
			c.Locals("auth_mode", "jwt")
		} else if prefix == "" || strings.HasPrefix(raw, prefix) {
			c.Locals("auth_mode", "apikey")
		}
		return c.Next()
	}
}

func registerOneEntry(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, apiKeyValidator func(ctx context.Context, key string) (*middleware.AuthContext, error), fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client, driver string) error {
	registerValidationMiddleware(app, entry, prefix)
	registerEntryRateLimit(app, entry, prefix)
	registerEntryTimeout(app, entry, prefix)

	mws := registerAuthMiddleware(entry, driver, jwtCfg, authValidator, apiKeyValidator, fgaClient, oryClient, zitadelClient)

	var err error
	switch entry.Type {
	case "crud":
		err = registerCRUD(app, entry, handlers, prefix, brokers, mws)
	case "rest":
		err = registerREST(app, entry, handlers, prefix, brokers, mws)
	case "webhook":
		err = registerREST(app, entry, handlers, prefix, brokers, mws)
	case "websocket":
		err = registerWebSocket(app, entry, handlers, prefix, mws)
	case "sse":
		err = registerSSE(app, entry, handlers, prefix, mws)
	case "file":
		err = registerFile(app, entry, handlers, prefix, brokers, mws)
	case "async":
		err = registerAsync(app, entry, handlers, prefix, mws)
	case "graphql":
		err = registerGraphQL(app, entry, handlers, prefix, models, mws)
	default:
		return fmt.Errorf("unknown entry type %q", entry.Type)
	}
	return err
}

func registerWithMws(app *fiber.App, method, path string, mws []fiber.Handler, h fiber.Handler) {
	registerRoute(app, method, path, h, toAnySlice(mws))
}

func registerRoute(app *fiber.App, method, path string, h fiber.Handler, mws []any) {
	switch method {
	case "GET":
		if len(mws) == 0 {
			app.Get(path, h)
		} else {
			app.Get(path, mws[0], append(mws[1:], h)...)
		}
	case "POST":
		if len(mws) == 0 {
			app.Post(path, h)
		} else {
			app.Post(path, mws[0], append(mws[1:], h)...)
		}
	case "PUT":
		if len(mws) == 0 {
			app.Put(path, h)
		} else {
			app.Put(path, mws[0], append(mws[1:], h)...)
		}
	case "PATCH":
		if len(mws) == 0 {
			app.Patch(path, h)
		} else {
			app.Patch(path, mws[0], append(mws[1:], h)...)
		}
	case "DELETE":
		if len(mws) == 0 {
			app.Delete(path, h)
		} else {
			app.Delete(path, mws[0], append(mws[1:], h)...)
		}
	default:
		app.Add([]string{method}, path, h, mws...)
	}
}

func toAnySlice(hs []fiber.Handler) []any {
	if len(hs) == 0 {
		return nil
	}
	all := make([]any, len(hs))
	for i, h := range hs {
		all[i] = h
	}
	return all
}

func jwtMiddleware(entry *EntryDef, jwtCfg *middleware.JWTConfig) fiber.Handler {
	if !hasAuth(entry, "jwt") || jwtCfg == nil {
		return nil
	}
	// Apply per-entry jwt_from override
	cfg := *jwtCfg
	if entry.JWTFrom != "" {
		cfg.TokenLookup = entry.JWTFrom
	}
	mw := middleware.JWT(cfg)
	return func(c fiber.Ctx) error {
		mode, _ := c.Locals("auth_mode").(string)
		if mode == "apikey" {
			return c.Next()
		}
		if mode == "" && !strings.HasPrefix(c.Get("Authorization"), "Bearer ") {
			return c.Next()
		}
		return mw(c)
	}
}

func zitadelJWTMiddleware(entry *EntryDef, jwtCfg *middleware.JWTConfig, zClient *zitadel.Client) fiber.Handler {
	if !hasAuth(entry, "jwt") || zClient == nil {
		return nil
	}
	return middleware.JWTWithZitadel(*jwtCfg, zClient)
}

func openfgaMiddleware(entry *EntryDef, fgaClient openfga.Checker, roles, permissions []string) fiber.Handler {
	if !hasAuth(entry, "jwt") || fgaClient == nil {
		return nil
	}
	return middleware.OpenFGA(middleware.OpenFGAConfig{
		Client:      fgaClient,
		Roles:       roles,
		Permissions: permissions,
	})
}

func oryMiddleware(entry *EntryDef, oryClient *ory.Client, roles, permissions []string) fiber.Handler {
	if !hasAuth(entry, "jwt") || oryClient == nil {
		return nil
	}
	return middleware.Ory(middleware.OryConfig{
		Client:      oryClient,
		Roles:       roles,
		Permissions: permissions,
	})
}

func manualAuthMiddleware(entry *EntryDef, roles, permissions []string, validator func(context.Context, *middleware.AuthContext, []string, []string) error) fiber.Handler {
	if !hasAuth(entry, "jwt") || validator == nil {
		return nil
	}
	return func(c fiber.Ctx) error {
		mode, _ := c.Locals("auth_mode").(string)
		if mode == "apikey" {
			return c.Next()
		}
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
	}
}

func apiKeyMiddleware(entry *EntryDef, apiKeyValidator func(ctx context.Context, key string) (*middleware.AuthContext, error), fgaClient openfga.Checker) fiber.Handler {
	object := fmt.Sprintf("%s:%s", entry.Type, entry.Path)
	mw := middleware.APIKey(middleware.APIKeyConfig{
		Prefix:       entry.APIPrefix,
		Client:       fgaClient,
		Relation:     "can_access",
		Object:       object,
		AuthResolver: apiKeyValidator,
	})
	return func(c fiber.Ctx) error {
		mode, _ := c.Locals("auth_mode").(string)
		if mode == "jwt" {
			return c.Next()
		}
		// If no router set a mode and header starts with Bearer, let JWT handle it
		if mode == "" && strings.HasPrefix(c.Get("Authorization"), "Bearer ") {
			return c.Next()
		}
		return mw(c)
	}
}

func apiKeyRoleMiddleware(entry *EntryDef, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error) []fiber.Handler {
	if !hasAuth(entry, "apikey") || len(entry.Roles) == 0 || authValidator == nil {
		return nil
	}
	return []fiber.Handler{func(c fiber.Ctx) error {
		if c.Locals("auth_mode") == "jwt" {
			return c.Next()
		}
		auth := middleware.GetAuth(c)
		if auth == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "auth context required",
			})
		}
		if err := authValidator(c.Context(), auth, entry.Roles, entry.Permissions); err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code":    403,
				"message": err.Error(),
			})
		}
		return c.Next()
	}}
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

func registerEntryTimeout(app *fiber.App, entry *EntryDef, prefix string) {
	if entry.Timeout == "" {
		return
	}
	d, err := time.ParseDuration(entry.Timeout)
	if err != nil {
		logx.Errorf("entry %s %s: invalid timeout %q, ignoring", entry.Type, entry.Path, entry.Timeout)
		return
	}
	app.Use(prefix+entry.Path, middleware.Timeout(d))
}

// fiber:context-methods migrated
