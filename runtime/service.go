package runtime

import (
	"context"
	"fmt"
	"maps"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/proc"
	"github.com/natuleadan/sdk-api/server"
	"github.com/natuleadan/sdk-api/server/auth/openfga"
	"github.com/natuleadan/sdk-api/server/auth/ory"
	"github.com/natuleadan/sdk-api/server/auth/zitadel"
	"github.com/natuleadan/sdk-api/server/middleware"
)

// Service is the main runtime orchestrator. It reads a service YAML,
// initializes databases, NATS connections, entry endpoints, and
// optionally exit workers and cron jobs.
type Service struct {
	config     *ServiceConfig
	srv        *server.Server
	pools      map[string]any
	natsConns  map[string]events.EventBroker
	handlers   *EntryHandlers
	hooks      map[string]any // model → EntryHooks[T]
	exitFuncs  map[string]ExitHandler
	exitHooks  map[string]ExitHooks
	exitMgr    *ExitWorkerManager
	cronSched  *CronScheduler
	cronFuncs  map[string]CronJobFunc
	models     map[string]*db.TableInfo
	safeClient *middleware.SafeHTTPClient
	jwtCfg     *middleware.JWTConfig
	fgaClient  openfga.Checker
	zitadelClient *zitadel.Client
	oryClient     *ory.Client
	authValidator func(context.Context, *middleware.AuthContext, []string) error

	stop context.CancelFunc
}

// New creates a Service from a YAML config file.
func New(configPath string) (*Service, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("runtime: %w", err)
	}
	return &Service{
		config:    cfg,
		pools:     make(map[string]any),
		natsConns: make(map[string]events.EventBroker),
		handlers:  &EntryHandlers{},
		exitMgr:   NewExitWorkerManager(),
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

// WithRest registers a REST handler by name.
func (s *Service) WithRest(name string, h func(*fiber.Ctx) error) *Service {
	if s.handlers.Rest == nil {
		s.handlers.Rest = make(map[string]func(*fiber.Ctx) error)
	}
	s.handlers.Rest[name] = h
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
// The validator receives the AuthContext and the YAML-defined roles for the entry.
// Return nil if allowed, an error with message if denied.
func (s *Service) WithAuthValidator(fn func(context.Context, *middleware.AuthContext, []string) error) *Service {
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

// PoolPG returns a *pgxpool.Pool by name.
func (s *Service) PoolPG(name string) any {
	return PoolPG(s.pools, name)
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

// Run starts the service: init DBs, NATS, register routes, start HTTP server.
func (s *Service) Run() error {
	return s.RunWithContext(context.Background())
}

// RunWithContext starts the service with a parent context.
func (s *Service) RunWithContext(ctx context.Context) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	s.stop = cancel

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
		s.srv.App().Static(sd.Prefix, sd.Dir)
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
		s.srv.App().Post(path, func(c *fiber.Ctx) error {
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
		if err := broker.Close(); err != nil { fmt.Fprintf(os.Stderr, "service: broker close error: %v\n", err) }
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
		ContextKey:  auth.ContextKey,
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

func initStorageFromDef(s *StorageDef) (server.StorageBackend, error) {
	switch s.Mode {
	case "local":
		return server.NewLocalStorage(s.Path)
	case "s3":
		return server.NewS3Storage(server.S3Config{
			Endpoint:        s.Endpoint,
			Region:          s.Region,
			Bucket:          s.Bucket,
			AccessKeyID:     s.AccessKey,
			SecretAccessKey: s.SecretKey,
		})
	default:
		return nil, fmt.Errorf("unsupported storage mode %q", s.Mode)
	}
}
