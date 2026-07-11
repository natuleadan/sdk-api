package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/collection"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/proc"
	"github.com/natuleadan/sdk-api/infra/stores/cache"
	"github.com/natuleadan/sdk-api/infra/stores/mon"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/natuleadan/sdk-api/infra/syncx"
	"github.com/natuleadan/sdk-api/server"
	"github.com/natuleadan/sdk-api/server/auth/openfga"
	"github.com/natuleadan/sdk-api/server/auth/ory"
	"github.com/natuleadan/sdk-api/server/auth/zitadel"
	"github.com/natuleadan/sdk-api/server/middleware"
)

// ErrNotFound is returned when a database record is not found.
var ErrNotFound = db.ErrNotFound

// Service is the main runtime orchestrator. It reads a service YAML,
// initializes databases, NATS connections, entry endpoints, and
// optionally exit workers and cron jobs.
type Service struct {
	config        *ServiceConfig
	srv           *server.Server
	pools         map[string]any
	natsConns     map[string]events.EventBroker
	handlers      *EntryHandlers
	hooks         map[string]any // model → EntryHooks[T]
	tables        map[string]any // model → *db.Table[T] (set by MustRegister)
	exitFuncs     map[string]ExitHandler
	exitHooks     map[string]ExitHooks
	exitMgr       *ExitWorkerManager
	cronSched     *CronScheduler
	cronFuncs     map[string]CronJobFunc
	models        map[string]*db.TableInfo
	safeClient    *middleware.SafeHTTPClient
	jwtCfg        *middleware.JWTConfig
	fgaClient     openfga.Checker
	zitadelClient *zitadel.Client
	oryClient     *ory.Client
	authValidator func(context.Context, *middleware.AuthContext, []string, []string) error

	stop context.CancelFunc
}

// New creates a Service from a YAML config file path.
func New(configPath string) (*Service, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("runtime: %w", err)
	}
	return newFromConfig(cfg)
}

// NewFromYAML creates a Service from in-memory YAML content (e.g. //go:embed).
func NewFromYAML(content []byte) (*Service, error) {
	cfg, err := ParseConfig(content)
	if err != nil {
		return nil, fmt.Errorf("runtime: %w", err)
	}
	return newFromConfig(cfg)
}

func newFromConfig(cfg *ServiceConfig) (*Service, error) {
	return &Service{
		config:    cfg,
		pools:     make(map[string]any),
		natsConns: make(map[string]events.EventBroker),
		handlers:  &EntryHandlers{},
		exitMgr:   NewExitWorkerManager(),
		tables:    make(map[string]any),
	}, nil
}

// WithHandlers registers all entry handler functions.
func (s *Service) WithHandlers(h *EntryHandlers) *Service {
	s.handlers = h
	return s
}

// WithCRUD registers a CRUD provider for a model name.
func (s *Service) WithCRUD(model string, provider CRUDProvider) *Service {
	if s.handlers.CRUD == nil {
		s.handlers.CRUD = make(map[string]CRUDProvider)
	}
	s.handlers.CRUD[model] = provider
	return s
}

// WithCRUDFactory registers a lazy CRUD provider factory.
// The factory is called once on the first HTTP request, after Run() has
// initialized all resources (database pools, NATS connections, etc.).
func (s *Service) WithCRUDFactory(model string, factory CRUDFactory) *Service {
	if s.handlers.CRUD == nil {
		s.handlers.CRUD = make(map[string]CRUDProvider)
	}
	s.handlers.CRUD[model] = &lazyCRUD{factory: factory}
	return s
}

// MustRegister auto-creates the table and registers a CRUDProvider for the model.
// The pool, table, and hooks are lazily initialized on the first HTTP request.
func MustRegister[T any](svc *Service, name, poolName, tableName string, hooks EntryHooks[T]) {
	svc.WithCRUDFactory(name, func() CRUDProvider {
		pool, ok := svc.pools[poolName].(*pgxpool.Pool)
		if !ok {
			log.Fatalf("runtime: pool %q not found or not a pgxpool", poolName)
		}
		tbl, err := db.NewTable[T](pool, tableName)
		if err != nil {
			log.Fatalf("runtime: new table %s: %v", name, err)
		}
		if err := tbl.AutoInit(context.Background()); err != nil {
			log.Fatalf("runtime: autoinit %s: %v", name, err)
		}
		svc.tables[name] = tbl
		return NewCRUDProvider(tbl, hooks)
	})
}

// MySQLMustRegister is like MustRegister but uses MySQL (*sql.DB) instead of PostgreSQL.
func MySQLMustRegister[T any](svc *Service, name, poolName, tableName string, hooks EntryHooks[T]) {
	svc.WithCRUDFactory(name, func() CRUDProvider {
		pool, ok := svc.pools[poolName].(*sql.DB)
		if !ok {
			log.Fatalf("runtime: mysql pool %q not found", poolName)
		}
		tbl, err := db.NewMySQLTable[T](pool, tableName)
		if err != nil {
			log.Fatalf("runtime: new mysql table %s: %v", name, err)
		}
		if err := tbl.AutoInit(context.Background()); err != nil {
			log.Fatalf("runtime: autoinit %s: %v", name, err)
		}
		return NewMySQLCRUDProvider(tbl, hooks)
	})
}

// CachedCRUD registers a CRUD provider with automatic L1 (memory) + L2 (Redis/Dragonfly) cache-aside.
// Cache is populated on miss using the DB primary key lookup. List/Create/Update/Delete return 405.
// The redisConf points to Dragonfly or Redis (NodeType or ClusterType).
// If l1TTL > 0, an in-process L1 cache (collection.Cache) is added in front of L2 for sub-μs reads.
func CachedCRUD[T any](svc *Service, name, poolName, tableName string,
	redisConf redis.RedisConf, keyPrefix string, l2TTL time.Duration, l1TTL time.Duration) {

	var l1 *collection.Cache
	if l1TTL > 0 {
		var err error
		l1, err = collection.NewCache(l1TTL, collection.WithLimit(10000))
		if err != nil {
			log.Fatalf("runtime: l1 cache %s: %v", name, err)
		}
	}

	cc := cache.NewNode(
		redis.MustNewRedis(redisConf),
		syncx.NewSingleFlight(),
		&cache.Stat{},
		db.ErrNotFound,
		cache.WithExpiry(l2TTL),
	)

	svc.WithCRUDFactory(name, func() CRUDProvider {
		pool, ok := svc.pools[poolName].(*pgxpool.Pool)
		if !ok {
			log.Fatalf("runtime: pool %q not found or not a pgxpool", poolName)
		}
		tbl, err := db.NewTable[T](pool, tableName)
		if err != nil {
			log.Fatalf("runtime: new table %s: %v", name, err)
		}
		if err := tbl.AutoInit(context.Background()); err != nil {
			log.Fatalf("runtime: autoinit %s: %v", name, err)
		}
		return &cachedCRUD[T]{table: tbl, l2: cc, l1: l1, keyPrefix: keyPrefix}
	})
}

type cachedCRUD[T any] struct {
	table     *db.Table[T]
	l2        cache.Cache
	l1        *collection.Cache
	keyPrefix string
}

func (c *cachedCRUD[T]) Get(fc fiber.Ctx, id string) error {
	key := c.keyPrefix + id

	if c.l1 != nil {
		if val, ok := c.l1.Get(key); ok {
			return fc.JSON(val)
		}
	}

	var val T
	err := c.l2.TakeWithExpireCtx(fc.Context(), &val, key,
		func(v any, expire time.Duration) error {
			found, err := c.table.Get(fc.Context(), id)
			if err != nil {
				return err
			}
			reflect.ValueOf(v).Elem().Set(reflect.ValueOf(*found))
			return nil
		})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fc.Status(404).JSON(map[string]any{"code": 404, "message": "link not found"})
		}
		return fc.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
	}

	if c.l1 != nil {
		c.l1.Set(key, val)
	}

	return fc.JSON(val)
}

func (c *cachedCRUD[T]) List(fc fiber.Ctx, params ListParams) error {
	return fc.SendStatus(405)
}

func (c *cachedCRUD[T]) Create(fc fiber.Ctx, body []byte) error {
	return fc.SendStatus(405)
}

func (c *cachedCRUD[T]) Update(fc fiber.Ctx, id string, body []byte) error {
	return fc.SendStatus(405)
}

func (c *cachedCRUD[T]) Delete(fc fiber.Ctx, id string) error {
	return fc.SendStatus(405)
}

// MySQLCachedCRUD registers a CRUD provider with L1+L2 cache using MySQL as DB backend.
// Identical to CachedCRUD but uses *sql.DB and db.NewMySQLTable internally.
func MySQLCachedCRUD[T any](svc *Service, name, poolName, tableName string,
	redisConf redis.RedisConf, keyPrefix string, l2TTL time.Duration, l1TTL time.Duration) {

	var l1 *collection.Cache
	if l1TTL > 0 {
		var err error
		l1, err = collection.NewCache(l1TTL, collection.WithLimit(10000))
		if err != nil {
			log.Fatalf("runtime: l1 cache %s: %v", name, err)
		}
	}

	cc := cache.NewNode(
		redis.MustNewRedis(redisConf),
		syncx.NewSingleFlight(),
		&cache.Stat{},
		db.ErrNotFound,
		cache.WithExpiry(l2TTL),
	)

	svc.WithCRUDFactory(name, func() CRUDProvider {
		sqlPool, ok := svc.pools[poolName].(*sql.DB)
		if !ok {
			log.Fatalf("runtime: pool %q not found or not a *sql.DB", poolName)
		}
		tbl, err := db.NewMySQLTable[T](sqlPool, tableName)
		if err != nil {
			log.Fatalf("runtime: new mysql table %s: %v", name, err)
		}
		if err := tbl.AutoInit(context.Background()); err != nil {
			log.Fatalf("runtime: autoinit %s: %v", name, err)
		}
		return &mysqlCachedCRUD[T]{table: tbl, l2: cc, l1: l1, keyPrefix: keyPrefix}
	})
}

type mysqlCachedCRUD[T any] struct {
	table     *db.MySQLTable[T]
	l2        cache.Cache
	l1        *collection.Cache
	keyPrefix string
}

func (c *mysqlCachedCRUD[T]) Get(fc fiber.Ctx, id string) error {
	key := c.keyPrefix + id

	if c.l1 != nil {
		if val, ok := c.l1.Get(key); ok {
			return fc.JSON(val)
		}
	}

	var val T
	err := c.l2.TakeWithExpireCtx(fc.Context(), &val, key,
		func(v any, expire time.Duration) error {
			found, err := c.table.Get(fc.Context(), id)
			if err != nil {
				return err
			}
			reflect.ValueOf(v).Elem().Set(reflect.ValueOf(*found))
			return nil
		})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fc.Status(404).JSON(map[string]any{"code": 404, "message": "link not found"})
		}
		return fc.Status(500).JSON(map[string]any{"code": 500, "message": err.Error()})
	}

	if c.l1 != nil {
		c.l1.Set(key, val)
	}

	return fc.JSON(val)
}

func (c *mysqlCachedCRUD[T]) List(fc fiber.Ctx, params ListParams) error {
	return fc.SendStatus(405)
}

func (c *mysqlCachedCRUD[T]) Create(fc fiber.Ctx, body []byte) error {
	return fc.SendStatus(405)
}

func (c *mysqlCachedCRUD[T]) Update(fc fiber.Ctx, id string, body []byte) error {
	return fc.SendStatus(405)
}

func (c *mysqlCachedCRUD[T]) Delete(fc fiber.Ctx, id string) error {
	return fc.SendStatus(405)
}

// TursoMustRegister registers a CRUD provider for Turso/SQLite backend.
func TursoMustRegister[T any](svc *Service, name, poolName, tableName string, hooks EntryHooks[T]) {
	svc.WithCRUDFactory(name, func() CRUDProvider {
		pool, ok := svc.pools[poolName].(*sql.DB)
		if !ok {
			log.Fatalf("runtime: turso pool %q not found", poolName)
		}
		tbl, err := db.NewTursoTableFrom[T](pool, tableName, nil)
		if err != nil {
			log.Fatalf("runtime: new turso table %s: %v", name, err)
		}
		if err := tbl.AutoInit(context.Background()); err != nil {
			log.Fatalf("runtime: autoinit %s: %v", name, err)
		}
		return NewTursoCRUDProvider(tbl, hooks)
	})
}

// MongoMustRegister registers a CRUD provider for MongoDB backend.
// The model is lazily initialized on the first HTTP request.
// lookupField is the document field used for Get (e.g. "_id" or "short_code").
func MongoMustRegister(svc *Service, name, poolName, database, collection, lookupField string) {
	svc.WithCRUDFactory(name, func() CRUDProvider {
		uri, ok := svc.pools[poolName].(string)
		if !ok {
			log.Fatalf("runtime: mongo pool %q not found", poolName)
		}
		model := mon.MustNewModel(uri, database, collection)
		return NewMongoCRUDProvider(model, lookupField)
	})
}

// WithRest registers a REST handler by name.
func (s *Service) WithRest(name string, h func(*RestCtx) error) *Service {
	if s.handlers.Rest == nil {
		s.handlers.Rest = make(map[string]func(fiber.Ctx) error)
	}
	s.handlers.Rest[name] = func(c fiber.Ctx) error { return h(newRestCtx(c)) }
	return s
}

// WithWS registers a WebSocket handler by name.
func (s *Service) WithWS(name string, h WSHandler) *Service {
	if s.handlers.WS == nil {
		s.handlers.WS = make(map[string]WSHandler)
	}
	s.handlers.WS[name] = h
	return s
}

// WithSSE registers an SSE handler by name.
func (s *Service) WithSSE(name string, h SSEHandler) *Service {
	if s.handlers.SSE == nil {
		s.handlers.SSE = make(map[string]SSEHandler)
	}
	s.handlers.SSE[name] = h
	return s
}

// WithHooks registers entry hooks for a model. The hooks are applied to the
// corresponding CRUD provider if one has been registered for that model.
func (s *Service) WithHooks(model string, hooks any) *Service {
	if s.hooks == nil {
		s.hooks = make(map[string]any)
	}
	s.hooks[model] = hooks
	if s.handlers.CRUD != nil {
		if provider, ok := s.handlers.CRUD[model]; ok {
			if setter, ok := provider.(interface{ SetHooks(any) }); ok {
				setter.SetHooks(hooks)
			}
		}
	}
	return s
}

// WithExit registers an exit handler by name (for NATS workers).
func (s *Service) WithExit(name string, h ExitHandler) *Service {
	if s.exitFuncs == nil {
		s.exitFuncs = make(map[string]ExitHandler)
	}
	s.exitFuncs[name] = h
	return s
}

// WithExitHooks registers exit hooks by worker name.
func (s *Service) WithExitHooks(h map[string]ExitHooks) *Service {
	s.exitHooks = h
	return s
}

// WithCron registers a cron handler by name (for mode=handler).
func (s *Service) WithCron(name string, handler CronJobFunc) *Service {
	if s.cronFuncs == nil {
		s.cronFuncs = make(map[string]CronJobFunc)
	}
	s.cronFuncs[name] = handler
	return s
}

// WithAsync registers an async job handler by name.
func (s *Service) WithAsync(name string, handler AsyncHandler) *Service {
	if s.handlers.Async == nil {
		s.handlers.Async = make(map[string]AsyncHandler)
	}
	s.handlers.Async[name] = handler
	return s
}

// WithAuthValidator registers a custom authorization validator for "manual" auth mode.
// The validator receives the AuthContext, YAML-defined roles, and YAML-defined permissions.
// Return nil if allowed, an error with message if denied.
func (s *Service) WithAuthValidator(fn func(context.Context, *middleware.AuthContext, []string, []string) error) *Service {
	s.authValidator = fn
	return s
}

// RegisterValidation registers a validation model by name for input validation.
// Usage: svc.RegisterValidation("CreateProduct", CreateProductInput{}).
func (s *Service) RegisterValidation(name string, model any) *Service {
	middleware.RegisterValidation(name, model)
	return s
}

// RegisterModel registers a model for OpenAPI schema generation.
// Usage: svc.RegisterModel("Product", (*Product)(nil)).
func (s *Service) RegisterModel(name string, model any) *Service {
	if s.models == nil {
		s.models = make(map[string]*db.TableInfo)
	}
	t := reflect.TypeOf(model)
	if t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		logx.Errorf("RegisterModel %s: model must be a struct pointer", name)
		return s
	}
	info, err := db.ParseStructReflect(t)
	if err != nil {
		logx.Errorf("RegisterModel %s: %v", name, err)
		return s
	}
	s.models[name] = info
	return s
}

// Pool returns a DB pool by name.
func (s *Service) Pool(name string) any {
	return s.pools[name]
}

// PoolPG returns a *pgxpool.Pool by name (as any).
func (s *Service) PoolPG(name string) any {
	return PoolPG(s.pools, name)
}

// PoolPGTyped returns a *pgxpool.Pool by name, or nil if not found.
func (s *Service) PoolPGTyped(name string) *pgxpool.Pool {
	return PoolPG(s.pools, name)
}

// Table returns a *db.Table[T] by model name (registered via MustRegister).
func (s *Service) Table(name string) any {
	if s.tables == nil {
		return nil
	}
	return s.tables[name]
}

// GetTable returns a typed *db.Table[T] for a model registered via MustRegister.
func GetTable[T any](s *Service, name string) *db.Table[T] {
	if s.tables == nil {
		return nil
	}
	tbl, _ := s.tables[name].(*db.Table[T])
	return tbl
}

// NATS returns a event broker connection by name.
func (s *Service) NATS(name string) events.EventBroker {
	return s.natsConns[name]
}

// SafeHTTPClient returns an SSRF-protected HTTP client if configured.
func (s *Service) SafeHTTPClient() *middleware.SafeHTTPClient {
	if s.safeClient == nil {
		return nil
	}
	return s.safeClient
}

// App returns the underlying Fiber app.
func (s *Service) App() *fiber.App {
	if s.srv == nil {
		return nil
	}
	return s.srv.App()
}

// Storage returns the storage backend registered for a given entry path.
func (s *Service) Storage(path string) server.StorageBackend {
	if s.handlers == nil || s.handlers.Storage == nil {
		return nil
	}
	return s.handlers.Storage[path]
}

// Run starts the service: init DBs, NATS, register routes, start HTTP server.
func (s *Service) Run() error {
	return s.RunWithContext(context.Background())
}

// RunWithContext starts the service with a parent context.
func (s *Service) RunWithContext(ctx context.Context) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	s.stop = cancel

	if err := validateConfigDeploy(s.config); err != nil {
		return fmt.Errorf("deploy validation: %w", err)
	}
	CheckVercelWarnings(s.config)

	if err := s.initDatabases(ctx); err != nil {
		return err
	}
	if err := s.initEventStreams(ctx); err != nil {
		return err
	}
	s.initSSRF()
	s.initServer()

	if err := s.registerEntryRoutes(); err != nil {
		return err
	}
	s.serveStaticFiles()
	registerDocs(s.srv.App(), s.config, s.models)

	if err := s.startExitWorkers(ctx); err != nil {
		return err
	}
	if err := s.startCron(ctx); err != nil {
		return err
	}

	logx.Infof("%s starting on :%d", s.config.Name, s.config.Port)
	proc.AddShutdownListener(func() { s.shutdown() })
	return s.srv.Start()
}

func (s *Service) initDatabases(ctx context.Context) error {
	if len(s.config.Databases) == 0 {
		return nil
	}
	pools, err := initDatabases(ctx, s.config.Databases)
	if err != nil {
		return fmt.Errorf("databases: %w", err)
	}
	s.pools = pools
	return nil
}

func (s *Service) initEventStreams(ctx context.Context) error {
	if len(s.config.EventStreams) > 0 {
		brokers, err := initEventStreams(ctx, s.config.EventStreams)
		if err != nil {
			return fmt.Errorf("event_streams: %w", err)
		}
		maps.Copy(s.natsConns, brokers)
	}
	return nil
}

func (s *Service) initSSRF() {
	if sc := s.config.Server.SSRF; sc != nil && sc.Enabled {
		s.safeClient = middleware.NewSafeHTTPClient(*convertSSRF(sc))
	}
}

func (s *Service) registerEntryRoutes() error {
	if len(s.config.Entry) == 0 {
		return nil
	}
	if s.handlers.Storage == nil {
		s.handlers.Storage = make(map[string]server.StorageBackend)
	}
	for _, entry := range s.config.Entry {
		if entry.Type == "file" && entry.Storage != nil {
			if s.handlers.Storage[entry.Path] == nil {
				backend, err := initStorageFromDef(entry.Storage)
				if err != nil {
					return fmt.Errorf("storage %s: %w", entry.Path, err)
				}
				s.handlers.Storage[entry.Path] = backend
				logx.Infof("storage ready: %s mode=%s", entry.Path, entry.Storage.Mode)
			}
		}
	}
	return RegisterEntries(s.srv.App(), s.config, s.handlers, s.config.Server.APIPrefix, s.natsConns, s.models, s.jwtCfg, s.authValidator, s.fgaClient, s.oryClient, s.zitadelClient)
}

func (s *Service) serveStaticFiles() {
	for _, sd := range s.config.Server.Static {
		dir := sd.Dir
		prefix := sd.Prefix
		s.srv.App().Get(prefix+"/*", func(c fiber.Ctx) error {
			path := filepath.Join(dir, fiber.Params[string](c, "*"))
			return c.SendFile(path, fiber.SendFile{})
		})
	}
}

func (s *Service) startExitWorkers(ctx context.Context) error {
	if len(s.config.Exit) == 0 {
		return nil
	}
	return s.exitMgr.Start(ctx, s.config.Exit, s.natsConns, s.exitFuncs, s.exitHooks)
}

func (s *Service) startCron(ctx context.Context) error {
	if len(s.config.Cron) == 0 {
		return nil
	}
	s.cronSched = NewCronScheduler()
	if err := s.cronSched.AddAll(ctx, s.config.Cron, s.natsConns, s.cronFuncs); err != nil {
		return fmt.Errorf("cron: %w", err)
	}
	s.cronSched.Start()
	return nil
}

func (s *Service) initServer() {
	sc := s.config.Server

	var corsCfg *server.CORSConfig
	if sc.CORS != nil {
		corsCfg = &server.CORSConfig{
			Origins:     sc.CORS.Origins,
			Methods:     sc.CORS.Methods,
			Headers:     sc.CORS.Headers,
			Credentials: sc.CORS.Credentials,
			MaxAge:      sc.CORS.MaxAge,
		}
	}

	var routes []server.RouteConfig
	for _, mw := range sc.Middleware {
		routes = append(routes, server.RouteConfig{
			Path:       mw.Path,
			Middleware: mw.Apply,
		})
	}

	srvCfg := server.Config{
		Port:            s.config.Port,
		Host:            sc.Host,
		Prefork:         sc.Prefork,
		BodyLimit:       sc.BodyLimit,
		Timeout:         parseServerDuration(sc.Timeout, 30*time.Second),
		MaxConns:        sc.MaxConns,
		MaxBytes:        sc.MaxBytes,
		MetricsPath:     sc.MetricsPath,
		HealthPath:      sc.HealthPath,
		ShutdownTimeout: parseServerDuration(sc.ShutdownTimeout, 10*time.Second),
		RecoverStack:    sc.RecoverStack,
		APIPrefix:       sc.APIPrefix,
		Routes:          routes,
		SecurityHeaders: convertSecurityHeaders(sc.SecurityHeaders),
		CSRF:            convertCSRF(sc.CSRF, sc.Cookies),
		RateLimit:       convertRateLimit(sc.RateLimit),
		TLS:             convertTLS(sc.TLS),
		SSRF:            convertSSRF(sc.SSRF),
	}

	s.srv = server.New(srvCfg, server.TelemetryConfig{}, securityConfig(sc), corsCfg)

	s.jwtCfg = buildJWTCfg(s.config.Auth)

	auth := s.config.Auth
	if auth != nil && auth.Enabled && auth.Driver != "none" {
		initAuthClients(s, auth)
	}

	// Auto-register CSP report endpoint if configured
	if sc.SecurityHeaders != nil && sc.SecurityHeaders.CSPReportPath != "" {
		path := sc.SecurityHeaders.CSPReportPath
		s.srv.App().Post(path, func(c fiber.Ctx) error {
			body := string(c.Body())
			logx.Errorf("CSP violation reported: %s", body)
			return c.SendStatus(204)
		})
		logx.Infof("CSP report endpoint registered at %s", path)
	}

}

func (s *Service) shutdown() {
	logx.Info("runtime: shutting down...")
	if s.stop != nil {
		s.stop()
	}
	if s.cronSched != nil {
		s.cronSched.Stop()
	}
	if s.exitMgr != nil {
		s.exitMgr.Shutdown(5 * time.Second)
	}
	for name, broker := range s.natsConns {
		if err := broker.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "service: broker close error: %v\n", err)
		}
		logx.Infof("nats %s drained", name)
	}
	for name, pool := range s.pools {
		if closer, ok := pool.(interface{ Close() }); ok {
			closer.Close()
			logx.Infof("pool %s closed", name)
		}
	}
}

func parseServerDuration(s string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return fallback
}

func initEventStreams(ctx context.Context, configs []EventStreamConnConf) (map[string]events.EventBroker, error) {
	brokers := make(map[string]events.EventBroker, len(configs))
	for i, cfg := range configs {
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("event_streams[%d] (%s): %w", i, cfg.Name, err)
		}
		var broker events.EventBroker
		switch cfg.Driver {
		case "nats":
			conn, connErr := events.Connect(ctx, events.ConnOptions{
				Name:          cfg.Name,
				URL:           cfg.URL,
				MaxReconnects: cfg.MaxReconnects,
				ReconnectWait: parseServerDuration(cfg.ReconnectWait, 2*time.Second),
				Timeout:       parseServerDuration(cfg.Timeout, 5*time.Second),
				RetryOnFail:   cfg.RetryOnFail,
			})
			if connErr != nil {
				return nil, fmt.Errorf("%s: %w", cfg.Name, connErr)
			}
			for _, sd := range cfg.Streams {
				sc := events.DefaultStreamConfig(sd.Name)
				if sd.MaxAge != "" {
					if d, durErr := time.ParseDuration(sd.MaxAge); durErr == nil {
						sc.MaxAge = d
					}
				}
				if sd.MaxBytes > 0 {
					sc.MaxBytes = sd.MaxBytes
				}
				sc.Storage = parseNATSStorage(sd.Storage)
				sc.Compression = parseNATSCompression(sd.Compression)
				if err := conn.EnsureStream(sc); err != nil {
					return nil, fmt.Errorf("%s: stream %s: %w", cfg.Name, sd.Name, err)
				}
			}
			broker = conn
		case "kafka":
			consumerGroup := cfg.ConsumerGroup
			if consumerGroup == "" {
				consumerGroup = cfg.Name + "-group"
			}
			broker = events.NewKafkaBroker(cfg.Name, cfg.Brokers, consumerGroup)
		}
		brokers[cfg.Name] = broker
		logx.Infof("event stream ready: %s driver=%s", cfg.Name, cfg.Driver)
	}
	return brokers, nil
}

func parseNATSStorage(s string) events.StorageType {
	switch s {
	case "memory":
		return events.MemoryStorage
	default:
		return events.FileStorage
	}
}

func parseNATSCompression(s string) events.CompressionType {
	switch s {
	case "none":
		return events.NoCompression
	default:
		return events.S2Compression
	}
}

func securityConfig(sc ServerConf) server.SecurityConfig {
	cfg := server.SecurityConfig{}
	if sc.Security != nil {
		if sc.Security.ContentSecurity != nil && sc.Security.ContentSecurity.Enabled {
			cfg.ContentSecurity = &server.ContentSecurityConf{
				Enabled:   sc.Security.ContentSecurity.Enabled,
				Strict:    sc.Security.ContentSecurity.Strict,
				PublicKey: sc.Security.ContentSecurity.PublicKey,
			}
		}
		if sc.Security.Cryption != nil && sc.Security.Cryption.Enabled {
			cfg.Cryption = &server.CryptionConf{
				Enabled: sc.Security.Cryption.Enabled,
				Key:     sc.Security.Cryption.Key,
			}
		}
	}
	return cfg
}

func buildJWTCfg(auth *AuthConfig) *middleware.JWTConfig {
	if auth == nil {
		return nil
	}
	return &middleware.JWTConfig{
		Secret:      auth.Secret,
		PrevSecret:  auth.PrevSecret,
		Algorithm:   auth.Algorithm,
		TokenLookup: auth.TokenLookup,
		Issuer:      auth.Issuer,
		Audience:    auth.Audience,
	}
}

func initAuthClients(s *Service, auth *AuthConfig) {
	switch auth.Driver {
	case "openfga-zitadel":
		if auth.OpenFGAURL != "" {
			fgaClient, err := openfga.NewClient(openfga.Config{
				APIURL:  auth.OpenFGAURL,
				StoreID: auth.OpenFGAStore,
			})
			if err != nil {
				logx.Errorf("auth: failed to create OpenFGA client: %v", err)
			} else {
				s.fgaClient = fgaClient
				seedOpenFGAPermissions(s, fgaClient)
				logx.Infof("auth: OpenFGA client initialized (%s)", auth.OpenFGAURL)
			}
		}
		if auth.ZitadelURL != "" {
			s.zitadelClient = zitadel.NewClient(zitadel.Config{Issuer: auth.ZitadelURL})
			logx.Infof("auth: Zitadel client initialized (%s)", auth.ZitadelURL)
		}
	case "ory":
		if auth.KratosURL != "" || auth.KetoURL != "" {
			s.oryClient = ory.NewClient(ory.Config{
				KratosPublicURL: auth.KratosURL,
				KetoURL:         auth.KetoURL,
			})
			logx.Infof("auth: Ory client initialized (kratos=%s, keto=%s)", auth.KratosURL, auth.KetoURL)
		}
	}
}

func seedOpenFGAPermissions(s *Service, client *openfga.Client) {
	permissions := collectPermissionsFromEntries(s.config)
	if len(permissions) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.SeedPermissions(ctx, permissions); err != nil {
		logx.Errorf("auth: failed to seed OpenFGA permissions: %v", err)
	} else {
		logx.Infof("auth: seeded %d OpenFGA permissions", len(permissions))
	}
}

func collectPermissionsFromEntries(cfg *ServiceConfig) []openfga.PermissionDef {
	if cfg == nil {
		return nil
	}
	seen := make(map[string]bool)
	var permissions []openfga.PermissionDef
	for _, entry := range cfg.Entry {
		if len(entry.Roles) == 0 {
			continue
		}
		resource := entry.Resource
		if resource == "" {
			resource = entry.Model
		}
		if resource == "" {
			continue
		}
		for _, role := range entry.Roles {
			if seen[role] {
				continue
			}
			seen[role] = true
			permissions = append(permissions, openfga.PermissionDef{
				Role:     role,
				Resource: resource,
				Actions:  defaultActionsForRole(role),
			})
		}
	}
	return permissions
}

func defaultActionsForRole(role string) []string {
	switch {
	case strings.HasSuffix(role, ":admin"), strings.HasSuffix(role, ":manager"):
		return []string{"create", "read", "update", "delete", "publish"}
	case strings.HasSuffix(role, ":editor"), strings.HasSuffix(role, ":writer"):
		return []string{"create", "read", "update"}
	case strings.HasSuffix(role, ":viewer"), strings.HasSuffix(role, ":reader"):
		return []string{"read"}
	default:
		return []string{"read"}
	}
}

func convertSecurityHeaders(cfg *SecurityHeadersConf) *middleware.SecurityHeadersConfig {
	if cfg == nil {
		return nil
	}
	return &middleware.SecurityHeadersConfig{
		FrameOptions:      cfg.FrameOptions,
		ReferrerPolicy:    cfg.ReferrerPolicy,
		PermissionsPolicy: cfg.PermissionsPolicy,
		HSTS:              cfg.HSTS,
		HSTSMaxAge:        cfg.HSTSMaxAge,
		HSTSIncludeSubs:   cfg.HSTSIncludeSubs,
		CSP:               cfg.CSP,
		COOP:              cfg.COOP,
		COEP:              cfg.COEP,
		CORP:              cfg.CORP,
		CacheControl:      cfg.CacheControl,
		CSPReportPath:     cfg.CSPReportPath,
	}
}

func convertCSRF(cfg *CSRFConf, cookieCfg *CookieConf) *middleware.CSRFConfig {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	c := &middleware.CSRFConfig{
		Enabled:      cfg.Enabled,
		CookieName:   cfg.CookieName,
		HeaderName:   cfg.HeaderName,
		SameSite:     cfg.SameSite,
		Secure:       cfg.Secure,
		ExcludePaths: cfg.ExcludePaths,
	}
	// Apply global cookie config if not overridden per-CSRF
	if c.SameSite == "" && cookieCfg != nil {
		c.SameSite = cookieCfg.SameSite
	}
	if !c.Secure && cookieCfg != nil {
		c.Secure = cookieCfg.Secure
	}
	return c
}

func convertRateLimit(cfg *RateLimitConf) *middleware.RateLimitConfig {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	var global, perIP, perUser *middleware.RateLimitEntry
	if cfg.Global != nil {
		global = &middleware.RateLimitEntry{
			RequestsPerSecond: cfg.Global.RequestsPerSecond,
			Burst:             cfg.Global.Burst,
		}
	}
	if cfg.PerIP != nil {
		perIP = &middleware.RateLimitEntry{
			RequestsPerSecond: cfg.PerIP.RequestsPerSecond,
			Burst:             cfg.PerIP.Burst,
		}
	}
	if cfg.PerUser != nil {
		perUser = &middleware.RateLimitEntry{
			RequestsPerSecond: cfg.PerUser.RequestsPerSecond,
			Burst:             cfg.PerUser.Burst,
		}
	}
	return &middleware.RateLimitConfig{
		Enabled:  cfg.Enabled,
		Driver:   cfg.Driver,
		RedisURL: cfg.RedisURL,
		Global:   global,
		PerIP:    perIP,
		PerUser:  perUser,
	}
}

func convertSSRF(cfg *SSRFConf) *middleware.SSRFConfig {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	return &middleware.SSRFConfig{
		Enabled:       cfg.Enabled,
		BlockPrivate:  cfg.BlockPrivate,
		BlockLoopback: cfg.BlockLoopback,
		BlockMetadata: cfg.BlockMetadata,
		AllowedHosts:  cfg.AllowedHosts,
	}
}

func convertTLS(cfg *TLSConf) *server.TLSConfig {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	tlsCfg := &server.TLSConfig{
		Enabled:      cfg.Enabled,
		MinVersion:   cfg.MinVersion,
		MaxVersion:   cfg.MaxVersion,
		CurvePrefs:   cfg.CurvePrefs,
		CipherSuites: cfg.CipherSuites,
		RedirectHTTP: cfg.RedirectHTTP,
		RedirectPort: cfg.RedirectPort,
	}
	if cfg.Manual != nil {
		tlsCfg.Manual = &server.ManualTLS{
			CertFile: cfg.Manual.CertFile,
			KeyFile:  cfg.Manual.KeyFile,
		}
	}
	if cfg.Autocert != nil {
		tlsCfg.Autocert = &server.AutocertTLS{
			Domains:  cfg.Autocert.Domains,
			Email:    cfg.Autocert.Email,
			CacheDir: cfg.Autocert.CacheDir,
		}
	}
	return tlsCfg
}

// cachedStorage wraps a StorageBackend with an optional L1 RAM cache.
// Writes go through to the backend. Reads check RAM first, then backend.
type cachedStorage struct {
	server.StorageBackend
	l1       *collection.Cache
	path     string // disk cache path (empty = no disk cache)
	presigner server.Presigner
}

func newCachedStorage(backend server.StorageBackend, cfg *CacheConfig, ttl time.Duration) (*cachedStorage, error) {
	cs := &cachedStorage{StorageBackend: backend}
	if p, ok := backend.(server.Presigner); ok {
		cs.presigner = p
	}
	if cfg.L1 == "ram" {
		var err error
		cs.l1, err = collection.NewCache(ttl, collection.WithLimit(cfg.L1Size))
		if err != nil {
			return nil, fmt.Errorf("l1 cache: %w", err)
		}
	}
	if cfg.L2 == "disk" && cfg.L2Path != "" {
		cs.path = cfg.L2Path
		if err := os.MkdirAll(cs.path, 0750); err != nil {
			return nil, fmt.Errorf("l2 cache dir: %w", err)
		}
	}
	return cs, nil
}

func (c *cachedStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if c.l1 != nil {
		if raw, ok := c.l1.Get(key); ok {
			if data, ok := raw.([]byte); ok {
				return io.NopCloser(bytes.NewReader(data)), nil
			}
		}
	}
	if c.path != "" {
		safeKey := sanitizeKey(key)
		fsys := os.DirFS(c.path)
		f, err := fsys.Open(safeKey)
		if err == nil {
			defer func() { if cerr := f.Close(); cerr != nil { logx.Errorf("cachedStorage close: %v", cerr) } }()
			data, rErr := io.ReadAll(f)
			if rErr == nil {
				if c.l1 != nil {
					c.l1.Set(key, data)
				}
				return io.NopCloser(bytes.NewReader(data)), nil
			}
		}
	}
	r, err := c.StorageBackend.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { if cerr := r.Close(); cerr != nil { logx.Errorf("cachedStorage close: %v", cerr) } }()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	// Store in caches asynchronously
	go func() {
		if c.l1 != nil {
			c.l1.Set(key, data)
		}
		if c.path != "" {
			if wErr := os.WriteFile(filepath.Join(c.path, sanitizeKey(key)), data, 0600); wErr != nil {
				logx.Errorf("cachedStorage disk write: %v", wErr)
			}
		}
	}()
	return io.NopCloser(bytes.NewReader(data)), nil
}

func sanitizeKey(key string) string {
	return strings.ReplaceAll(key, "..", "")
}

func (c *cachedStorage) PresignURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if c.presigner != nil {
		return c.presigner.PresignURL(ctx, key, ttl)
	}
	return "", fmt.Errorf("underlying storage does not support presigned URLs")
}

func initStorageFromDef(s *StorageDef) (server.StorageBackend, error) {
	switch s.Mode {
	case "local":
		return server.NewLocalStorage(s.Path)
	case "s3":
		endpoint := s.Endpoint
		useSSL := false
		if endpoint != "" {
			if len(endpoint) > 8 && endpoint[:8] == "https://" {
				endpoint = endpoint[8:]
				useSSL = true
			} else if len(endpoint) > 7 && endpoint[:7] == "http://" {
				endpoint = endpoint[7:]
			}
		}
		var pool *server.PoolConfig
		if s.Pool != nil {
			dur, _ := time.ParseDuration(s.Pool.IdleTimeout)
			pool = &server.PoolConfig{
				MaxIdleConns:        s.Pool.MaxIdleConns,
				MaxIdleConnsPerHost: s.Pool.MaxIdlePerHost,
				MaxConnsPerHost:     s.Pool.MaxConnsPerHost,
				IdleTimeout:         dur,
			}
		}
		s3store, s3err := server.NewS3Storage(server.S3Config{
			Endpoint:        endpoint,
			Region:          s.Region,
			Bucket:          s.Bucket,
			AccessKeyID:     s.AccessKey,
			SecretAccessKey: s.SecretKey,
			UseSSL:          useSSL,
			Pool:            pool,
		})
		if s3err != nil {
			return nil, s3err
		}
		if s.Cache != nil && (s.Cache.L1 == "ram" || s.Cache.L2 == "disk") {
			ttl, _ := time.ParseDuration(s.Cache.L1TTL)
			if ttl <= 0 {
				ttl = 5 * time.Minute
			}
			return newCachedStorage(s3store, s.Cache, ttl)
		}
		return s3store, nil
	default:
		return nil, fmt.Errorf("unsupported storage mode %q", s.Mode)
	}
}
