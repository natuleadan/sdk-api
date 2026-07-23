package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/natuleadan/sdk-api/server"
	"github.com/natuleadan/sdk-api/server/auth/openfga"
	"github.com/natuleadan/sdk-api/server/auth/ory"
	"github.com/natuleadan/sdk-api/server/auth/zitadel"
	"github.com/natuleadan/sdk-api/server/middleware"
)

var versionRe = regexp.MustCompile(`/v\d+$`)

// buildEntryPrefix builds the full URL prefix for an entry.
// If the global prefix already contains a version (e.g. /api/v1),
// it is used as-is. Otherwise, the entry's api_version is appended.
func buildEntryPrefix(prefix string, entry *EntryDef) string {
	if versionRe.MatchString(prefix) {
		return prefix
	}
	if entry.APIVersion == "" {
		return prefix
	}
	return prefix + "/" + entry.APIVersion
}

var rlMaxFunc atomic.Value

func SetRateLimitMaxFunc(fn func(c fiber.Ctx) int) {
	rlMaxFunc.Store(fn)
}

func getRateLimitMaxFunc() func(c fiber.Ctx) int {
	fn, _ := rlMaxFunc.Load().(func(c fiber.Ctx) int)
	return fn
}

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

func RegisterEntries(app *fiber.App, cfg *ServiceConfig, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, apiKeyValidator func(ctx context.Context, key string) (*middleware.AuthContext, error), fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client, rlRdb ...*redis.Redis) error {
	return registerEntries(app, cfg, handlers, prefix, brokers, models, jwtCfg, authValidator, apiKeyValidator, fgaClient, oryClient, zitadelClient, nil, nil, rlRdb...)
}

func registerEntries(app *fiber.App, cfg *ServiceConfig, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, apiKeyValidator func(ctx context.Context, key string) (*middleware.AuthContext, error), fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client, pools map[string]any, kvConns map[string]*redis.Redis, rlRdb ...*redis.Redis) error {
	driver := ""
	if cfg.Auth != nil {
		driver = cfg.Auth.Driver
	}

	var serverPerUser, serverPerKey *middleware.RateLimitEntry
	var rlAlgorithm string
	var rlTTL time.Duration
	if cfg.Server.RateLimit != nil {
		rlAlgorithm = cfg.Server.RateLimit.Algorithm
		if cfg.Server.RateLimit.TTL != "" {
			rlTTL, _ = time.ParseDuration(cfg.Server.RateLimit.TTL)
		}
		if cfg.Server.RateLimit.PerUser != nil {
			serverPerUser = &middleware.RateLimitEntry{
				RequestsPerSecond: cfg.Server.RateLimit.PerUser.RequestsPerSecond,
				Burst:             cfg.Server.RateLimit.PerUser.Burst,
				TTL:               parseDurationDef(cfg.Server.RateLimit.PerUser.TTL),
			}
		}
		if cfg.Server.RateLimit.PerKey != nil {
			serverPerKey = &middleware.RateLimitEntry{
				RequestsPerSecond: cfg.Server.RateLimit.PerKey.RequestsPerSecond,
				Burst:             cfg.Server.RateLimit.PerKey.Burst,
				TTL:               parseDurationDef(cfg.Server.RateLimit.PerKey.TTL),
			}
		}
	}

	var rlRedis *redis.Redis
	if len(rlRdb) > 0 && rlRdb[0] != nil {
		rlRedis = rlRdb[0]
	}

	for i, entry := range cfg.Entry {
		if len(entry.AuthModes) > 0 {
			if err := validateEntryAuth(&entry, handlers); err != nil {
				return fmt.Errorf("entry[%d] %s:%s: %w", i, entry.Type, entry.Path, err)
			}
		}
		if err := registerOneEntry(app, &entry, handlers, prefix, brokers, models, jwtCfg, authValidator, apiKeyValidator, fgaClient, oryClient, zitadelClient, driver, serverPerUser, serverPerKey, rlAlgorithm, rlTTL, pools, kvConns, rlRedis); err != nil {
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

func registerAuthMiddleware(entry *EntryDef, driver string, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, apiKeyValidator func(ctx context.Context, key string) (*middleware.AuthContext, error), fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client, serverPerUser, serverPerKey *middleware.RateLimitEntry, rlAlgorithm string, rlTTL time.Duration, rlRdb ...*redis.Redis) []fiber.Handler {
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

	if hasJWT {
		mws = appendJWTMiddleware(mws, entry, driver, jwtCfg, authValidator, fgaClient, oryClient, zitadelClient)
		if entry.RequiresMFA {
			mws = append(mws, middleware.MFARequired())
		}
	}

	if hasAPIKey || hasJWT {
		if mw := buildPostAuthRL(serverPerUser, serverPerKey, entry, rlAlgorithm, rlTTL, rlRdb...); mw != nil {
			mws = append(mws, mw)
		}
	}

	return mws
}

func appendJWTMiddleware(mws []fiber.Handler, entry *EntryDef, driver string, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client) []fiber.Handler {
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
		if oryClient != nil {
			mws = append(mws, oryJWTMiddleware(entry, jwtCfg, oryClient))
		} else {
			mws = append(mws, jwtMiddleware(entry, jwtCfg))
		}
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

func buildPostAuthRL(serverPerUser, serverPerKey *middleware.RateLimitEntry, entry *EntryDef, rlAlgorithm string, rlTTL time.Duration, rlRdb ...*redis.Redis) fiber.Handler {
	cfg := middleware.RateLimitPostConfig{
		ServerPerUser: serverPerUser,
		ServerPerKey:  serverPerKey,
		MaxFunc:       getRateLimitMaxFunc(),
		Algorithm:     rlAlgorithm,
		TTL:           rlTTL,
	}
	if len(rlRdb) > 0 {
		cfg.RedisConn = rlRdb[0]
	}
	if entry.RateLimitPerUser != nil {
		cfg.EntryPerUser = &middleware.RateLimitEntry{
			RequestsPerSecond: entry.RateLimitPerUser.RequestsPerSecond,
			Burst:             entry.RateLimitPerUser.Burst,
			TTL:               parseDurationDef(entry.RateLimitPerUser.TTL),
		}
	}
	if entry.RateLimitPerKey != nil {
		cfg.EntryPerKey = &middleware.RateLimitEntry{
			RequestsPerSecond: entry.RateLimitPerKey.RequestsPerSecond,
			Burst:             entry.RateLimitPerKey.Burst,
			TTL:               parseDurationDef(entry.RateLimitPerKey.TTL),
		}
	}
	if len(entry.PerRoleLimits) > 0 {
		cfg.PerRoleLimits = make(map[string]*middleware.RateLimitEntry, len(entry.PerRoleLimits))
		for role, def := range entry.PerRoleLimits {
			if def != nil {
				cfg.PerRoleLimits[role] = &middleware.RateLimitEntry{
					RequestsPerSecond: def.RequestsPerSecond,
					Burst:             def.Burst,
					TTL:               parseDurationDef(def.TTL),
				}
			}
		}
	}
	if cfg.ServerPerUser == nil && cfg.ServerPerKey == nil && cfg.EntryPerUser == nil && cfg.EntryPerKey == nil && len(cfg.PerRoleLimits) == 0 {
		return nil
	}
	return middleware.RateLimitPost(cfg)
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

func registerOneEntry(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, models map[string]*db.TableInfo, jwtCfg *middleware.JWTConfig, authValidator func(context.Context, *middleware.AuthContext, []string, []string) error, apiKeyValidator func(ctx context.Context, key string) (*middleware.AuthContext, error), fgaClient openfga.Checker, oryClient *ory.Client, zitadelClient *zitadel.Client, driver string, serverPerUser, serverPerKey *middleware.RateLimitEntry, rlAlgorithm string, rlTTL time.Duration, pools map[string]any, kvConns map[string]*redis.Redis, rlRdb ...*redis.Redis) error {
	versionPrefix := buildEntryPrefix(prefix, entry)

	registerDeprecation(app, entry, versionPrefix)
	registerValidationMiddleware(app, entry, versionPrefix)
	registerEntryRateLimit(app, entry, versionPrefix, rlAlgorithm, rlTTL, rlRdb...)
	registerEntryTimeout(app, entry, versionPrefix)
	registerRetry(app, entry, versionPrefix)
	registerFallback(app, entry, versionPrefix)
	registerBulkhead(entry)

	mws := registerAuthMiddleware(entry, driver, jwtCfg, authValidator, apiKeyValidator, fgaClient, oryClient, zitadelClient, serverPerUser, serverPerKey, rlAlgorithm, rlTTL, rlRdb...)

	var err error
	switch entry.Type {
	case "crud":
		err = registerCRUD(app, entry, handlers, versionPrefix, brokers, mws)
	case "rest":
		err = registerREST(app, entry, handlers, versionPrefix, brokers, mws)
	case "webhook":
		err = registerREST(app, entry, handlers, versionPrefix, brokers, mws)
	case "websocket":
		err = registerWebSocket(app, entry, handlers, versionPrefix, mws)
	case "sse":
		err = registerSSE(app, entry, handlers, versionPrefix, mws)
	case "file":
		err = registerFile(app, entry, handlers, versionPrefix, brokers, mws)
	case "async":
		var store JobStore
		store, err = resolveAsyncStore(entry, pools, kvConns, brokers)
		if err == nil {
			err = registerAsync(app, entry, handlers, versionPrefix, mws, store)
		}
	case "grpc":
		err = registerGRPC(app, entry, handlers, versionPrefix, brokers, models, mws)
	case "graphql":
		err = registerGraphQL(app, entry, handlers, versionPrefix, models, mws)
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
	// When jwt_from overrides the default (header:Authorization), don't skip
	// based on Authorization header — token may be in cookie or query param.
	hasCustomLookup := entry.JWTFrom != ""
	mw := middleware.JWT(cfg)
	return func(c fiber.Ctx) error {
		mode, _ := c.Locals("auth_mode").(string)
		if mode == "apikey" {
			return c.Next()
		}
		if !hasCustomLookup && mode == "" && !strings.HasPrefix(c.Get("Authorization"), "Bearer ") {
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

func oryJWTMiddleware(entry *EntryDef, jwtCfg *middleware.JWTConfig, oClient *ory.Client) fiber.Handler {
	if !hasAuth(entry, "jwt") || oClient == nil {
		return nil
	}
	return middleware.JWTWithOry(*jwtCfg, oClient)
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

func registerBulkhead(entry *EntryDef) {
	for name, limit := range entry.Bulkhead {
		if limit > 0 {
			BulkheadRegister(name, limit)
		}
	}
}

func registerRetry(app *fiber.App, entry *EntryDef, prefix string) {
	if entry.Retry == nil || entry.Retry.MaxRetries <= 0 {
		return
	}
	initial, _ := time.ParseDuration(entry.Retry.InitialInterval)
	maxBackoff, _ := time.ParseDuration(entry.Retry.MaxBackoff)
	app.Use(prefix+entry.Path, middleware.Retry(middleware.RetryConfig{
		MaxRetries:      entry.Retry.MaxRetries,
		InitialInterval: initial,
		MaxBackoff:      maxBackoff,
		Multiplier:      entry.Retry.Multiplier,
	}))
}

func registerFallback(app *fiber.App, entry *EntryDef, prefix string) {
	if entry.Fallback == "" {
		return
	}
	app.Use(prefix+entry.Path, middleware.Fallback(middleware.FallbackConfig{
		Mode: entry.Fallback,
	}))
}

func registerDeprecation(app *fiber.App, entry *EntryDef, prefix string) {
	if entry.APIStatus == "" || entry.APIStatus == "current" {
		return
	}
	var sunset time.Time
	if entry.SunsetDate != "" {
		sunset, _ = time.Parse(time.RFC3339, entry.SunsetDate)
	}
	app.Use(prefix+entry.Path, middleware.Deprecation(middleware.DeprecationConfig{
		Status:     entry.APIStatus,
		SunsetDate: sunset,
	}))
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

func registerEntryRateLimit(app *fiber.App, entry *EntryDef, prefix, algorithm string, ttl time.Duration, kvRdb ...*redis.Redis) {
	if entry.RateLimit == nil || entry.RateLimit.RequestsPerSecond <= 0 {
		return
	}
	rlCfg := middleware.RateLimitConfig{
		Algorithm: algorithm,
		TTL:       ttl,
		MaxFunc:   getRateLimitMaxFunc(),
		Global: &middleware.RateLimitEntry{
			RequestsPerSecond: entry.RateLimit.RequestsPerSecond,
			Burst:             entry.RateLimit.Burst,
			TTL:               parseDurationDef(entry.RateLimit.TTL),
		},
	}
	if len(kvRdb) > 0 {
		rlCfg.RedisConn = kvRdb[0]
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
