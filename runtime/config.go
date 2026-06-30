package runtime

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/natuleadan/sdk-api/infra/conf"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// ---- Top-level ----

type ServiceConfig struct {
	Name         string                `json:"name"`
	Port         int                   `json:"port,default=8080"`
	Server       ServerConf            `json:"server,optional"`
	Databases    []DBConfig            `json:"databases,optional"`
	NATS         []NATSConnConf        `json:"nats,optional"`
	EventStreams []EventStreamConnConf `json:"event_streams,optional"`
	Entry        []EntryDef            `json:"entry,optional"`
	Exit         []ExitWorker          `json:"exit,optional"`
	Cron         []CronJob             `json:"cron,optional"`
}

// ---- Server ----

type ServerConf struct {
	Host            string                `json:"host,default=0.0.0.0"`
	Prefork         bool                  `json:"prefork,optional"`
	BodyLimit       int                   `json:"body_limit,default=4194304"`
	Timeout         string                `json:"timeout,default=30s"`
	MaxConns        int                   `json:"max_conns,default=1000"`
	MaxBytes        int                   `json:"max_bytes,default=4194304"`
	MetricsPath     string                `json:"metrics_path,default=/metrics"`
	HealthPath      string                `json:"health_path,default=/health"`
	ShutdownTimeout string                `json:"shutdown_timeout,default=10s"`
	RecoverStack    bool                  `json:"recover_stack,default=true"`
	APIPrefix       string                `json:"api_prefix,default=/api/v1"`
	CORS            *CORSConf             `json:"cors,optional"`
	Middleware      []RouteMW             `json:"middleware,optional"`
	Static          []StaticDef           `json:"static,optional"`
	MaxConnLimit    int                   `json:"max_conn_limit,default=1000"`
	OpenAPI         *OpenAPIConf          `json:"openapi,optional"`
	SecurityHeaders *SecurityHeadersConf  `json:"security_headers,optional"`
	CSRF            *CSRFConf             `json:"csrf,optional"`
	RateLimit       *RateLimitConf        `json:"rate_limit,optional"`
	TLS             *TLSConf              `json:"tls,optional"`
	SSRF            *SSRFConf             `json:"ssrf,optional"`
	Cookies         *CookieConf           `json:"cookies,optional"`
}

type CookieConf struct {
	SameSite string `json:"same_site,optional"` // Strict, Lax, None
	Secure   bool   `json:"secure,optional"`
}

type RateLimitConf struct {
	Enabled  bool            `json:"enabled,optional"`
	Driver   string          `json:"driver,default=memory"`
	RedisURL string          `json:"redis_url,optional"`
	Global   *RateLimitDef   `json:"global,optional"`
	PerIP    *RateLimitDef   `json:"per_ip,optional"`
	PerUser  *RateLimitDef   `json:"per_user,optional"`
}

type RateLimitDef struct {
	RequestsPerSecond int `json:"requests_per_second"`
	Burst             int `json:"burst"`
}

type SSRFConf struct {
	Enabled       bool     `json:"enabled,optional"`
	BlockPrivate  bool     `json:"block_private,optional"`
	BlockLoopback bool     `json:"block_loopback,optional"`
	BlockMetadata bool     `json:"block_metadata,optional"`
	AllowedHosts  []string `json:"allowed_hosts,optional"`
}

type TLSConf struct {
	Enabled      bool         `json:"enabled"`
	Manual       *ManualTLS   `json:"manual,optional"`
	Autocert     *AutocertTLS `json:"autocert,optional"`
	MinVersion   string       `json:"min_version,optional"`
	MaxVersion   string       `json:"max_version,optional"`
	CurvePrefs   []string     `json:"curve_preferences,optional"`
	CipherSuites []string     `json:"cipher_suites,optional"`
	RedirectHTTP bool         `json:"redirect_http,optional"`
	RedirectPort int          `json:"redirect_port,optional"`
}

type ManualTLS struct {
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

type AutocertTLS struct {
	Domains  []string `json:"domains"`
	Email    string   `json:"email"`
	CacheDir string   `json:"cache_dir,optional"`
}

type SecurityHeadersConf struct {
	FrameOptions      string `json:"frame_options,optional"`
	ReferrerPolicy    string `json:"referrer_policy,optional"`
	PermissionsPolicy string `json:"permissions_policy,optional"`
	HSTS              bool   `json:"hsts,optional"`
	HSTSMaxAge        int    `json:"hsts_max_age,optional"`
	HSTSIncludeSubs   bool   `json:"hsts_include_subdomains,optional"`
	CSP               string `json:"csp,optional"`
	COOP              string `json:"coop,optional"`
	COEP              string `json:"coep,optional"`
	CORP              string `json:"corp,optional"`
	CacheControl      string `json:"cache_control,optional"`
	CSPReportPath     string `json:"csp_report_path,optional"` // auto-register POST endpoint for CSP reports (e.g. "/csp-violation")
}

type CSRFConf struct {
	Enabled     bool     `json:"enabled,optional"`
	CookieName  string   `json:"cookie_name,optional"`
	HeaderName  string   `json:"header_name,optional"`
	SameSite    string   `json:"same_site,optional"`
	Secure      bool     `json:"secure,optional"`
	ExcludePaths []string `json:"exclude_paths,optional"`
}

type RouteMW struct {
	Path  string   `json:"path"`
	Apply []string `json:"apply"`
}

type CORSConf struct {
	Origins     []string `json:"origins,optional"`
	Methods     []string `json:"methods,optional"`
	Headers     []string `json:"headers,optional"`
	Credentials bool     `json:"credentials,optional"`
	MaxAge      int      `json:"max_age,default=300"`
}

type StaticDef struct {
	Prefix string `json:"prefix"`
	Dir    string `json:"dir"`
}

type OpenAPIConf struct {
	Enabled  bool   `json:"enabled,optional"`
	Version  string `json:"version,default=1.0.0"`
	SpecPath string `json:"spec_path,default=/openapi.json"`
	DocsPath string `json:"docs_path,default=/docs"`
	Theme    string `json:"theme,default=moon"`
	DarkMode bool   `json:"dark_mode,default=true"`
}

// ---- Database ----

type DBConfig struct {
	Name   string    `json:"name"`
	Driver string    `json:"driver,default=postgres"`
	URL    string    `json:"url"`
	Pool   *PoolConf `json:"pool,optional"`
}

type PoolConf struct {
	MaxConns          int32  `json:"max_conns,default=10"`
	MinConns          int32  `json:"min_conns,default=2"`
	MaxConnLifetime   string `json:"max_conn_lifetime,optional"`
	MaxConnIdleTime   string `json:"max_conn_idle_time,optional"`
	HealthCheckPeriod string `json:"health_check_period,optional"`
	ReservedConns     int32  `json:"reserved_conns,default=10"`
}

func (d *DBConfig) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if d.URL == "" {
		return fmt.Errorf("url is required")
	}
	if d.Driver == "" {
		d.Driver = "postgres"
	}
	switch d.Driver {
	case "postgres", "pg":
		d.Driver = "postgres"
	case "turso", "mysql":
	default:
		return fmt.Errorf("unknown driver %q (use postgres, turso, or mysql)", d.Driver)
	}
	if d.Pool == nil {
		d.Pool = &PoolConf{MaxConns: 10, MinConns: 2, ReservedConns: 10}
	}
	if d.Pool.MaxConns < 1 {
		d.Pool.MaxConns = 10
	}
	if d.Pool.MinConns < 1 {
		d.Pool.MinConns = 2
	}
	if d.Pool.ReservedConns < 1 {
		d.Pool.ReservedConns = 10
	}
	return nil
}

// ---- Event Streams (NATS + Kafka) ----

type EventStreamConnConf struct {
	Name          string      `json:"name"`
	Driver        string      `json:"driver"` // nats, kafka
	URL           string      `json:"url,optional"`
	Brokers       []string    `json:"brokers,optional"`
	ConsumerGroup string      `json:"consumer_group,optional"`
	MaxReconnects int         `json:"max_reconnects,optional"`
	ReconnectWait string      `json:"reconnect_wait,optional"`
	Timeout       string      `json:"timeout,optional"`
	RetryOnFail   bool        `json:"retry_on_fail,optional"`
	Streams       []StreamDef `json:"streams,optional"`
}

func (e *EventStreamConnConf) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch e.Driver {
	case "":
		e.Driver = "nats"
		fallthrough
	case "nats":
		if e.URL == "" {
			return fmt.Errorf("nats driver: url is required")
		}
		if e.MaxReconnects <= 0 {
			e.MaxReconnects = 10
		}
		if e.ReconnectWait == "" {
			e.ReconnectWait = "2s"
		}
		if e.Timeout == "" {
			e.Timeout = "5s"
		}
	case "kafka":
		if len(e.Brokers) == 0 {
			return fmt.Errorf("kafka driver: brokers is required")
		}
		if e.ConsumerGroup == "" {
			e.ConsumerGroup = e.Name + "-group"
		}
	default:
		return fmt.Errorf("unknown event stream driver %q (use nats or kafka)", e.Driver)
	}
	return nil
}

// ---- NATS (legacy) ----

type NATSConnConf struct {
	Name          string      `json:"name"`
	URL           string      `json:"url"`
	MaxReconnects int         `json:"max_reconnects,default=10"`
	ReconnectWait string      `json:"reconnect_wait,default=2s"`
	Timeout       string      `json:"timeout,default=5s"`
	RetryOnFail   bool        `json:"retry_on_fail,default=true"`
	Streams       []StreamDef `json:"streams,optional"`
}

type StreamDef struct {
	Name        string `json:"name"`
	MaxAge      string `json:"max_age,optional"`
	MaxBytes    int64  `json:"max_bytes,optional"`
	Storage     string `json:"storage,default=file"`
	Compression string `json:"compression,default=s2"`
}

func (n *NATSConnConf) Validate() error {
	if n.Name == "" {
		return fmt.Errorf("name is required")
	}
	if n.URL == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

// ---- Entry Endpoints ----

type EntryDef struct {
	Type    string `json:"type"` // crud, rest, webhook, websocket, sse, file
	Method  string `json:"method,optional"`
	Path    string `json:"path,optional"`
	Handler string `json:"handler,optional"`
	Auth    bool   `json:"auth,optional"`
	DB      string `json:"db,optional"` // references database name

	// CRUD
	Model     string         `json:"model,optional"`
	Table     string         `json:"table,optional"`
	Resource  string         `json:"resource,optional"`
	Overrides *CRUDOverrides `json:"overrides,optional"`

	// Event stream selection
	EventStream string `json:"event_stream,optional"`

	// Publish after entry — event_publish is the new name, nats_publish is legacy
	EventPublish []EventPublishTarget `json:"event_publish,optional"`
	NATSPublish  []EventPublishTarget `json:"nats_publish,optional"`

	// File
	AllowedTypes []string   `json:"allowed_types,optional"`
	MaxSize      string     `json:"max_size,optional"`
	MaxFiles     int        `json:"max_files,optional"`
	MagicBytes   bool       `json:"magic_bytes,optional"`
	Storage      *StorageDef `json:"storage,optional"`

	// Security per-entry overrides
	CSRF      *bool            `json:"csrf,optional"` // false = skip CSRF for this entry
	RateLimit *RateLimitDef    `json:"rate_limit,optional"` // per-entry rate limit

	// Validation
	ValidationModel string `json:"validate,optional"` // validation model name
}

type CRUDOverrides struct {
	List   string `json:"list,optional"`
	Get    string `json:"get,optional"`
	Create string `json:"create,optional"`
	Update string `json:"update,optional"`
	Delete string `json:"delete,optional"`
}

type NATSPublishTarget = EventPublishTarget

type EventPublishTarget struct {
	Stream      string `json:"stream"`
	Subject     string `json:"subject,optional"`
	EventStream string `json:"event_stream,optional"` // broker name; empty = all brokers
}

type StorageDef struct {
	Mode      string `json:"mode"` // s3, local
	Bucket    string `json:"bucket,optional"`
	Path      string `json:"path,optional"`
	Region    string `json:"region,optional"`
	Endpoint  string `json:"endpoint,optional"`
	AccessKey string `json:"access_key,optional"`
	SecretKey string `json:"secret_key,optional"`
}

func (e *EntryDef) Validate() error {
	if e.Type == "" {
		return fmt.Errorf("type is required")
	}
	switch e.Type {
	case "crud":
		if e.Model == "" {
			return fmt.Errorf("crud: model is required")
		}
		if e.DB == "" {
			return fmt.Errorf("crud: db is required")
		}
		if e.Table == "" {
			e.Table = toSnake(e.Model)
		}
		if e.Resource == "" {
			e.Resource = plural(e.Table)
		}
		if e.Path == "" {
			e.Path = "/" + e.Resource
		}
		return nil

	case "rest":
		if e.Method == "" {
			return fmt.Errorf("rest: method is required")
		}
		if e.Path == "" {
			return fmt.Errorf("rest: path is required")
		}
		if e.Handler == "" {
			return fmt.Errorf("rest: handler is required")
		}

	case "webhook":
		if e.Method == "" {
			e.Method = "POST"
		}
		if e.Path == "" {
			return fmt.Errorf("webhook: path is required")
		}
		if e.Handler == "" {
			return fmt.Errorf("webhook: handler is required")
		}

	case "websocket":
		if e.Path == "" {
			return fmt.Errorf("websocket: path is required")
		}
		if e.Handler == "" {
			return fmt.Errorf("websocket: handler is required")
		}

	case "sse":
		if e.Path == "" {
			return fmt.Errorf("sse: path is required")
		}
		if e.Handler == "" {
			return fmt.Errorf("sse: handler is required")
		}

	case "file":
		if e.Method == "" {
			return fmt.Errorf("file: method is required")
		}
		if e.Path == "" {
			return fmt.Errorf("file: path is required")
		}
		if e.Handler == "" {
			return fmt.Errorf("file: handler is required")
		}
		if e.Storage == nil {
			return fmt.Errorf("file: storage is required")
		}
		switch e.Storage.Mode {
		case "s3", "local":
		default:
			return fmt.Errorf("file: storage.mode must be s3 or local (got %q)", e.Storage.Mode)
		}
		if e.Storage.Mode == "local" && e.Storage.Path == "" {
			return fmt.Errorf("file: storage.path is required for mode=local")
		}

	case "async":
		if e.Path == "" {
			return fmt.Errorf("async: path is required")
		}
		if e.Handler == "" {
			return fmt.Errorf("async: handler is required")
		}

	case "graphql":
		if e.Path == "" {
			return fmt.Errorf("graphql: path is required")
		}

	default:
		return fmt.Errorf("unknown entry type %q (use crud, rest, webhook, websocket, sse, file, async, or graphql)", e.Type)
	}

	// Validate publish targets (check event_publish first, then nats_publish)
	targets := e.EventPublish
	if len(targets) == 0 {
		targets = e.NATSPublish
	}
	for _, p := range targets {
		if p.Stream == "" {
			return fmt.Errorf("event_publish: stream is required")
		}
		if p.Subject == "" {
			p.Subject = p.Stream
		}
	}

	return nil
}

// ---- Exit Workers ----

type ExitWorker struct {
	Name          string       `json:"name"`
	Subscribe     SubscribeDef `json:"subscribe"`
	Handler       string       `json:"handler"`
	MaxConcurrent int          `json:"max_concurrent,default=1"`
	DB            string       `json:"db,optional"`
	Reply         bool         `json:"reply,optional"`
	ReplyTimeout  string       `json:"reply_timeout,default=30s"`
	PullBatch     int          `json:"pull_batch,optional"`
	PullMaxWait   string       `json:"pull_max_wait,optional"`
	ConsumerMode  string       `json:"consumer_mode,optional"` // push or pull
	EventStream   string       `json:"event_stream,optional"` // broker name
}

type SubscribeDef struct {
	Stream  string `json:"stream"`
	Subject string `json:"subject,optional"`
	Durable string `json:"durable,optional"`
}

func (e *ExitWorker) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("name is required")
	}
	if e.Subscribe.Stream == "" {
		return fmt.Errorf("subscribe.stream is required")
	}
	if e.Handler == "" {
		return fmt.Errorf("handler is required")
	}
	if e.Subscribe.Subject == "" {
		e.Subscribe.Subject = e.Subscribe.Stream
	}
	if e.Subscribe.Durable == "" {
		e.Subscribe.Durable = e.Name + "-worker"
	}
	if e.MaxConcurrent < 1 {
		e.MaxConcurrent = 1
	}
	if e.ReplyTimeout == "" {
		e.ReplyTimeout = "30s"
	}
	return nil
}

// ---- Cron ----

type CronJob struct {
	Name     string       `json:"name"`
	Schedule string       `json:"schedule"`
	Mode     string       `json:"mode,default=nats"` // nats, handler, internal
	Publish  *CronPublish `json:"publish,optional"`
	Handler  string       `json:"handler,optional"`
}

type CronPublish struct {
	Stream  string `json:"stream"`
	Subject string `json:"subject,optional"`
}

func (c *CronJob) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.Schedule == "" {
		return fmt.Errorf("schedule is required")
	}
	if c.Mode == "" {
		c.Mode = "nats"
	}
	switch c.Mode {
	case "nats":
		if c.Publish == nil || c.Publish.Stream == "" {
			return fmt.Errorf("mode=nats: publish.stream is required")
		}
		if c.Publish.Subject == "" {
			c.Publish.Subject = c.Publish.Stream
		}
	case "handler":
		if c.Handler == "" {
			return fmt.Errorf("mode=handler: handler is required")
		}
	case "internal":
		// nothing extra needed
	default:
		return fmt.Errorf("unknown cron mode %q (use nats, handler, or internal)", c.Mode)
	}
	return nil
}

// ---- LoadConfig ----

func LoadConfig(path string) (*ServiceConfig, error) {
	var cfg ServiceConfig
	if path != "" {
		if err := conf.Load(path, &cfg, conf.UseEnv()); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	// env override for port
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.Port = p
		}
	}

	// Validate databases
	seenDB := make(map[string]bool)
	for i := range cfg.Databases {
		if err := cfg.Databases[i].Validate(); err != nil {
			return nil, fmt.Errorf("databases[%d] (%s): %w", i, cfg.Databases[i].Name, err)
		}
		if seenDB[cfg.Databases[i].Name] {
			return nil, fmt.Errorf("databases[%d]: duplicate name %q", i, cfg.Databases[i].Name)
		}
		seenDB[cfg.Databases[i].Name] = true
	}

	// Validate NATS
	seenNATS := make(map[string]bool)
	for i := range cfg.NATS {
		if err := cfg.NATS[i].Validate(); err != nil {
			return nil, fmt.Errorf("nats[%d] (%s): %w", i, cfg.NATS[i].Name, err)
		}
		if seenNATS[cfg.NATS[i].Name] {
			return nil, fmt.Errorf("nats[%d]: duplicate name %q", i, cfg.NATS[i].Name)
		}
		seenNATS[cfg.NATS[i].Name] = true
	}

	// Validate Event Streams
	seenES := make(map[string]bool)
	for i := range cfg.EventStreams {
		if err := cfg.EventStreams[i].Validate(); err != nil {
			return nil, fmt.Errorf("event_streams[%d] (%s): %w", i, cfg.EventStreams[i].Name, err)
		}
		if seenES[cfg.EventStreams[i].Name] {
			return nil, fmt.Errorf("event_streams[%d]: duplicate name %q", i, cfg.EventStreams[i].Name)
		}
		if seenNATS[cfg.EventStreams[i].Name] {
			return nil, fmt.Errorf("event_streams[%d]: name %q conflicts with nats entry", i, cfg.EventStreams[i].Name)
		}
		seenES[cfg.EventStreams[i].Name] = true
	}

	// Validate entry endpoints
	for i := range cfg.Entry {
		if err := cfg.Entry[i].Validate(); err != nil {
			return nil, fmt.Errorf("entry[%d] (%s %s): %w", i, cfg.Entry[i].Type, cfg.Entry[i].Path, err)
		}
		// Validate DB reference exists (skip if no databases declared)
		if cfg.Entry[i].DB != "" && !seenDB[cfg.Entry[i].DB] && len(cfg.Databases) > 0 {
			return nil, fmt.Errorf("entry[%d] (%s): db %q not found in databases", i, cfg.Entry[i].Path, cfg.Entry[i].DB)
		}
	}

	// Validate exit workers
	for i := range cfg.Exit {
		if err := cfg.Exit[i].Validate(); err != nil {
			return nil, fmt.Errorf("exit[%d] (%s): %w", i, cfg.Exit[i].Name, err)
		}
		if cfg.Exit[i].DB != "" && !seenDB[cfg.Exit[i].DB] && len(cfg.Databases) > 0 {
			return nil, fmt.Errorf("exit[%d] (%s): db %q not found in databases", i, cfg.Exit[i].Name, cfg.Exit[i].DB)
		}
	}

	// Validate cron
	for i := range cfg.Cron {
		if err := cfg.Cron[i].Validate(); err != nil {
			return nil, fmt.Errorf("cron[%d] (%s): %w", i, cfg.Cron[i].Name, err)
		}
	}

	// Warn about potential plaintext secrets in config
	checkPlaintextSecrets(&cfg)

	return &cfg, nil
}

// checkPlaintextSecrets logs warnings for values that look like secrets
// but are hardcoded instead of using ${VAR} environment variable substitution.
func checkPlaintextSecrets(cfg *ServiceConfig) {
	secretFields := map[string]func(*ServiceConfig) string{
		"JWT secret":        func(c *ServiceConfig) string { return "" }, // checked via server.auth
		"Databases URL":     func(c *ServiceConfig) string {
			if len(cfg.Databases) > 0 {
				return cfg.Databases[0].URL
			}
			return ""
		},
	}
	_ = secretFields

	// Check database URLs
	for i, db := range cfg.Databases {
		if looksLikePlaintextSecret(db.URL) {
			logx.Errorf("config: databases[%d].url appears to contain a plaintext secret (use ${VAR} instead)", i)
		}
	}

	// Check NATS URLs
	for i, n := range cfg.NATS {
		if looksLikePlaintextSecret(n.URL) {
			logx.Errorf("config: nats[%d].url appears to contain a plaintext secret (use ${VAR} instead)", i)
		}
	}

	// Check storage secrets (S3 access/secret keys)
	for i, entry := range cfg.Entry {
		if entry.Storage != nil {
			if looksLikePlaintextSecret(entry.Storage.AccessKey) {
				logx.Errorf("config: entry[%d].storage.access_key appears to be a plaintext secret", i)
			}
			if looksLikePlaintextSecret(entry.Storage.SecretKey) {
				logx.Errorf("config: entry[%d].storage.secret_key appears to be a plaintext secret", i)
			}
		}
	}
}

func looksLikePlaintextSecret(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		return false
	}
	// Heuristic: if it contains common credential patterns, warn
	lower := strings.ToLower(s)
	if strings.Contains(lower, "password") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "key") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "auth") {
		return true
	}
	// If it looks like a JWT or base64 (long string with dots or many chars)
	if len(s) > 40 && (strings.Contains(s, ".") || strings.Count(s, "") > 60) {
		return true
	}
	return false
}

// ---- helpers ----

var specialToSnake = map[string]string{
	"ID":        "id",
	"URL":       "url",
	"API":       "api",
	"JSON":      "json",
	"XML":       "xml",
	"HTML":      "html",
	"SQL":       "sql",
	"SSH":       "ssh",
	"UUID":      "uuid",
	"JWT":       "jwt",
	"NATS":      "nats",
	"HTTP":      "http",
	"DB":        "db",
	"WS":        "ws",
	"SSE":       "sse",
}

func toSnake(s string) string {
	if mapped, ok := specialToSnake[s]; ok {
		return mapped
	}
	if len(s) <= 1 {
		return s
	}
	// Check if the first two are uppercase and the rest is not -> treat as acronym
	// e.g., "IPAddr" -> "ip_addr"
	result := make([]byte, 0, len(s)+4)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i > 0 && c >= 'A' && c <= 'Z' {
			prev := s[i-1]
			next := byte(0)
			if i+1 < len(s) {
				next = s[i+1]
			}
			if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z' && next >= 'a' && next <= 'z') {
				result = append(result, '_')
			}
		}
		if c >= 'A' && c <= 'Z' {
			result = append(result, c+32)
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

func plural(s string) string {
	if s == "" {
		return ""
	}
	if s[len(s)-1] == 's' {
		return s
	}
	if s[len(s)-1] == 'y' && len(s) > 1 && s[len(s)-2] != 'a' && s[len(s)-2] != 'e' && s[len(s)-2] != 'i' && s[len(s)-2] != 'o' && s[len(s)-2] != 'u' {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}
