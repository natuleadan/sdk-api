package runtime

import (
	"fmt"
	"os"
	"strconv"

	"github.com/natuleadan/sdk-api/infra/conf"
)

// ---- Top-level ----

type ServiceConfig struct {
	Name      string         `json:"name"`
	Port      int            `json:"port,default=8080"`
	Server    ServerConf     `json:"server,optional"`
	Databases []DBConfig     `json:"databases,optional"`
	NATS      []NATSConnConf `json:"nats,optional"`
	Entry     []EntryDef     `json:"entry,optional"`
	Exit      []ExitWorker   `json:"exit,optional"`
	Cron      []CronJob      `json:"cron,optional"`
}

// ---- Server ----

type ServerConf struct {
	Host            string      `json:"host,default=0.0.0.0"`
	Prefork         bool        `json:"prefork,optional"`
	BodyLimit       int         `json:"body_limit,default=4194304"`
	Timeout         string      `json:"timeout,default=30s"`
	MaxConns        int         `json:"max_conns,default=1000"`
	MaxBytes        int         `json:"max_bytes,default=4194304"`
	MetricsPath     string      `json:"metrics_path,default=/metrics"`
	HealthPath      string      `json:"health_path,default=/health"`
	ShutdownTimeout string      `json:"shutdown_timeout,default=10s"`
	RecoverStack    bool        `json:"recover_stack,default=true"`
	APIPrefix       string      `json:"api_prefix,default=/api/v1"`
	CORS            *CORSConf   `json:"cors,optional"`
	Middleware      []RouteMW   `json:"middleware,optional"`
	Static          []StaticDef `json:"static,optional"`
	MaxConnLimit    int         `json:"max_conn_limit,default=1000"`
	OpenAPI         *OpenAPIConf `json:"openapi,optional"`
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

// ---- NATS ----

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

	// NATS publish after entry
	NATSPublish []NATSPublishTarget `json:"nats_publish,optional"`

	// File
	AllowedTypes []string   `json:"allowed_types,optional"`
	MaxSize      string     `json:"max_size,optional"`
	Storage      *StorageDef `json:"storage,optional"`
}

type CRUDOverrides struct {
	List   string `json:"list,optional"`
	Get    string `json:"get,optional"`
	Create string `json:"create,optional"`
	Update string `json:"update,optional"`
	Delete string `json:"delete,optional"`
}

type NATSPublishTarget struct {
	Stream  string `json:"stream"`
	Subject string `json:"subject,optional"`
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

	// Validate NATS publish targets
	for _, p := range e.NATSPublish {
		if p.Stream == "" {
			return fmt.Errorf("nats_publish: stream is required")
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

	return &cfg, nil
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
