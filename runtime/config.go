package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"os/exec"
	"strconv"
	"strings"

	"github.com/natuleadan/sdk-api/infra/conf"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// ---- Top-level ----

type DeployConfig struct {
	Target string `json:"target" config:",default=auto"`
}

type ServiceConfig struct {
	Name         string                `json:"name"`
	Port         int                   `json:"port" config:",default=8080"`
	Deploy       *DeployConfig         `json:"deploy" config:",optional"`
	Server       ServerConf            `json:"server" config:",optional"`
	Databases    []DBConfig            `json:"databases" config:",optional"`
	EventStreams []EventStreamConnConf `json:"event_streams" config:",optional"`
	Entry        []EntryDef            `json:"entry" config:",optional"`
	Exit         []ExitWorker          `json:"exit" config:",optional"`
	Cron         []CronJob             `json:"cron" config:",optional"`
	Auth         *AuthConfig           `json:"auth" config:",optional"`
}

type AuthConfig struct {
	Enabled     bool   `json:"enabled" config:",optional"`
	Driver      string `json:"driver" config:",default=none"` // none | manual | openfga-zitadel | ory
	Secret      string `json:"secret" config:",optional"`
	PrevSecret  string `json:"prev_secret" config:",optional"`
	Algorithm   string `json:"algorithm" config:",default=HS256"`
	TokenLookup string `json:"token_lookup" config:",default=header:Authorization"`
	ContextKey  string `json:"context_key" config:",default=claims"`
	Issuer      string `json:"issuer" config:",optional"`
	Audience    string `json:"audience" config:",optional"`
	Expiry      int    `json:"expiry" config:",default=3600"`
	ZitadelURL  string `json:"zitadel_url" config:",optional"`
	OpenFGAURL  string `json:"openfga_url" config:",optional"`
	OpenFGAStore string `json:"openfga_store" config:",optional"`
	KratosURL   string `json:"kratos_url" config:",optional"`
	KetoURL     string `json:"keto_url" config:",optional"`
}

// ---- Server ----

type ServerConf struct {
	Host            string               `json:"host" config:",default=0.0.0.0"`
	Prefork         bool                 `json:"prefork" config:",optional"`
	BodyLimit       int                  `json:"body_limit" config:",default=4194304"`
	Timeout         string               `json:"timeout" config:",default=30s"`
	MaxConns        int                  `json:"max_conns" config:",default=1000"`
	MaxBytes        int                  `json:"max_bytes" config:",default=4194304"`
	MetricsPath     string               `json:"metrics_path" config:",default=/metrics"`
	HealthPath      string               `json:"health_path" config:",default=/health"`
	ShutdownTimeout string               `json:"shutdown_timeout" config:",default=10s"`
	RecoverStack    bool                 `json:"recover_stack" config:",default=true"`
	APIPrefix       string               `json:"api_prefix" config:",default=/api/v1"`
	CORS            *CORSConf            `json:"cors" config:",optional"`
	Middleware      []RouteMW            `json:"middleware" config:",optional"`
	Static          []StaticDef          `json:"static" config:",optional"`
	MaxConnLimit    int                  `json:"max_conn_limit" config:",default=1000"`
	OpenAPI         *OpenAPIConf         `json:"openapi" config:",optional"`
	SecurityHeaders *SecurityHeadersConf `json:"security_headers" config:",optional"`
	CSRF            *CSRFConf            `json:"csrf" config:",optional"`
	RateLimit       *RateLimitConf       `json:"rate_limit" config:",optional"`
	TLS             *TLSConf             `json:"tls" config:",optional"`
	SSRF            *SSRFConf            `json:"ssrf" config:",optional"`
	Cookies         *CookieConf          `json:"cookies" config:",optional"`
	Security        *SecurityDef         `json:"security" config:",optional"`
	Logger          bool                 `json:"logger" config:",default=true"`
	LoadShedding    bool                 `json:"load_shedding" config:",default=true"`
	Breaker         bool                 `json:"breaker" config:",default=true"`
}

type SecurityDef struct {
	ContentSecurity *ContentSecurityDef `json:"content_security" config:",optional"`
	Cryption        *CryptionDef        `json:"cryption" config:",optional"`
}

type ContentSecurityDef struct {
	Enabled   bool   `json:"enabled" config:",optional"`
	Strict    bool   `json:"strict" config:",optional"`
	PublicKey string `json:"public_key"`
}

type CryptionDef struct {
	Enabled bool   `json:"enabled" config:",optional"`
	Key     string `json:"key"`
}

type CookieConf struct {
	SameSite string `json:"same_site" config:",optional"` // Strict, Lax, None
	Secure   bool   `json:"secure" config:",optional"`
}

type RateLimitConf struct {
	Enabled  bool          `json:"enabled" config:",optional"`
	Driver   string        `json:"driver" config:",default=memory"`
	RedisURL string        `json:"redis_url" config:",optional"`
	Global   *RateLimitDef `json:"global" config:",optional"`
	PerIP    *RateLimitDef `json:"per_ip" config:",optional"`
	PerUser  *RateLimitDef `json:"per_user" config:",optional"`
}

type RateLimitDef struct {
	RequestsPerSecond int `json:"requests_per_second"`
	Burst             int `json:"burst"`
}

type SSRFConf struct {
	Enabled       bool     `json:"enabled" config:",optional"`
	BlockPrivate  bool     `json:"block_private" config:",optional"`
	BlockLoopback bool     `json:"block_loopback" config:",optional"`
	BlockMetadata bool     `json:"block_metadata" config:",optional"`
	AllowedHosts  []string `json:"allowed_hosts" config:",optional"`
}

type TLSConf struct {
	Enabled      bool         `json:"enabled"`
	Manual       *ManualTLS   `json:"manual" config:",optional"`
	Autocert     *AutocertTLS `json:"autocert" config:",optional"`
	MinVersion   string       `json:"min_version" config:",optional"`
	MaxVersion   string       `json:"max_version" config:",optional"`
	CurvePrefs   []string     `json:"curve_preferences" config:",optional"`
	CipherSuites []string     `json:"cipher_suites" config:",optional"`
	RedirectHTTP bool         `json:"redirect_http" config:",optional"`
	RedirectPort int          `json:"redirect_port" config:",optional"`
}

type ManualTLS struct {
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

type AutocertTLS struct {
	Domains  []string `json:"domains"`
	Email    string   `json:"email"`
	CacheDir string   `json:"cache_dir" config:",optional"`
}

type SecurityHeadersConf struct {
	FrameOptions      string `json:"frame_options" config:",optional"`
	ReferrerPolicy    string `json:"referrer_policy" config:",optional"`
	PermissionsPolicy string `json:"permissions_policy" config:",optional"`
	HSTS              bool   `json:"hsts" config:",optional"`
	HSTSMaxAge        int    `json:"hsts_max_age" config:",optional"`
	HSTSIncludeSubs   bool   `json:"hsts_include_subdomains" config:",optional"`
	CSP               string `json:"csp" config:",optional"`
	COOP              string `json:"coop" config:",optional"`
	COEP              string `json:"coep" config:",optional"`
	CORP              string `json:"corp" config:",optional"`
	CacheControl      string `json:"cache_control" config:",optional"`
	CSPReportPath     string `json:"csp_report_path" config:",optional"` // auto-register POST endpoint for CSP reports (e.g. "/csp-violation")
}

type CSRFConf struct {
	Enabled      bool     `json:"enabled" config:",optional"`
	CookieName   string   `json:"cookie_name" config:",optional"`
	HeaderName   string   `json:"header_name" config:",optional"`
	SameSite     string   `json:"same_site" config:",optional"`
	Secure       bool     `json:"secure" config:",optional"`
	ExcludePaths []string `json:"exclude_paths" config:",optional"`
}

type RouteMW struct {
	Path  string   `json:"path"`
	Apply []string `json:"apply"`
}

type CORSConf struct {
	Origins     []string `json:"origins" config:",optional"`
	Methods     []string `json:"methods" config:",optional"`
	Headers     []string `json:"headers" config:",optional"`
	Credentials bool     `json:"credentials" config:",optional"`
	MaxAge      int      `json:"max_age" config:",default=300"`
}

type StaticDef struct {
	Prefix string `json:"prefix"`
	Dir    string `json:"dir"`
}

type OpenAPIConf struct {
	Enabled  bool   `json:"enabled" config:",optional"`
	Version  string `json:"version" config:",default=1.0.0"`
	SpecPath string `json:"spec_path" config:",default=/openapi.json"`
	DocsPath string `json:"docs_path" config:",default=/docs"`
	Theme    string `json:"theme" config:",default=moon"`
	DarkMode bool   `json:"dark_mode" config:",default=true"`
}

// ---- Database ----

type DBConfig struct {
	Name     string      `json:"name"`
	Driver   string      `json:"driver" config:",default=postgres"`
	URL      string      `json:"url"`
	Database string      `json:"database" config:",optional"`
	Pool     *PoolConf   `json:"pool" config:",optional"`
	Turso    *TursoConf  `json:"turso" config:",optional"`
}

type TursoConf struct {
	Mode        string `json:"mode" config:",default=local"`         // local | remote
	BusyTimeout int    `json:"busy_timeout" config:",default=30000"` // ms, 0 = no wait
}

func (t *TursoConf) Validate() error {
	if t.Mode != "local" && t.Mode != "remote" {
		return fmt.Errorf("turso mode %q invalid (use local or remote)", t.Mode)
	}
	if t.BusyTimeout < 0 {
		t.BusyTimeout = 30000
	}
	return nil
}

type PoolConf struct {
	MaxConns          int32  `json:"max_conns" config:",default=10"`
	MinConns          int32  `json:"min_conns" config:",default=2"`
	MaxConnLifetime   string `json:"max_conn_lifetime" config:",optional"`
	MaxConnIdleTime   string `json:"max_conn_idle_time" config:",optional"`
	HealthCheckPeriod string `json:"health_check_period" config:",optional"`
	ReservedConns     int32  `json:"reserved_conns" config:",default=10"`
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
	case "turso":
		if d.Turso == nil {
			d.Turso = &TursoConf{Mode: "local", BusyTimeout: 30000}
		}
		if err := d.Turso.Validate(); err != nil {
			return err
		}
	case "mysql", "mongo":
	default:
		return fmt.Errorf("unknown driver %q (use postgres, turso, mysql, or mongo)", d.Driver)
	}
	if d.Driver == "mongo" {
		return nil
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
	URL           string      `json:"url" config:",optional"`
	Brokers       []string    `json:"brokers" config:",optional"`
	ConsumerGroup string      `json:"consumer_group" config:",optional"`
	MaxReconnects int         `json:"max_reconnects" config:",optional"`
	ReconnectWait string      `json:"reconnect_wait" config:",optional"`
	Timeout       string      `json:"timeout" config:",optional"`
	RetryOnFail   bool        `json:"retry_on_fail" config:",optional"`
	Streams       []StreamDef `json:"streams" config:",optional"`
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

type StreamDef struct {
	Name        string `json:"name"`
	MaxAge      string `json:"max_age" config:",optional"`
	MaxBytes    int64  `json:"max_bytes" config:",optional"`
	Storage     string `json:"storage" config:",default=file"`
	Compression string `json:"compression" config:",default=s2"`
}

// ---- Entry Endpoints ----

type EntryDef struct {
	Type    string `json:"type"` // crud, rest, webhook, websocket, sse, file
	Method  string `json:"method" config:",optional"`
	Path    string `json:"path" config:",optional"`
	Handler string `json:"handler" config:",optional"`
	Auth    bool   `json:"auth" config:",optional"`
	Roles   []string `json:"roles" config:",optional"`
	Permissions []string `json:"permissions" config:",optional"`
	DB      string `json:"db" config:",optional"` // references database name

	// CRUD
	Model     string         `json:"model" config:",optional"`
	Table     string         `json:"table" config:",optional"`
	Resource  string         `json:"resource" config:",optional"`
	Overrides *CRUDOverrides `json:"overrides" config:",optional"`

	// Event stream selection
	EventStream string `json:"event_stream" config:",optional"`

	// Publish after entry — event_publish is the new name, nats_publish is legacy
	EventPublish []EventPublishTarget `json:"event_publish" config:",optional"`
	NATSPublish  []EventPublishTarget `json:"nats_publish" config:",optional"`

	// File
	AllowedTypes []string    `json:"allowed_types" config:",optional"`
	MaxSize      string      `json:"max_size" config:",optional"`
	MaxFiles     int         `json:"max_files" config:",optional"`
	MagicBytes   bool        `json:"magic_bytes" config:",optional"`
	Storage      *StorageDef `json:"storage" config:",optional"`

	// Security per-entry overrides
	CSRF      *bool         `json:"csrf" config:",optional"`       // false = skip CSRF for this entry
	RateLimit *RateLimitDef `json:"rate_limit" config:",optional"` // per-entry rate limit

	// Validation
	ValidationModel string `json:"validate" config:",optional"` // validation model name

	// Timeout per-entry (e.g. "30s")
	Timeout string `json:"timeout" config:",optional"`

	// Pagination (CRUD only)
	PageSize    int      `json:"page_size" config:",optional"`     // default 10, also min
	MaxPageSize int      `json:"max_page_size" config:",optional"` // default 100, also max
	Pagination  string   `json:"pagination" config:",optional"`    // "offset" | "keyset"
	Sortable    []string `json:"sortable" config:",optional"`      // allowed sort columns
}

type CRUDOverrides struct {
	List   string `json:"list" config:",optional"`
	Get    string `json:"get" config:",optional"`
	Create string `json:"create" config:",optional"`
	Update string `json:"update" config:",optional"`
	Delete string `json:"delete" config:",optional"`
}

type NATSPublishTarget = EventPublishTarget

type EventPublishTarget struct {
	Stream      string `json:"stream"`
	Subject     string `json:"subject" config:",optional"`
	EventStream string `json:"event_stream" config:",optional"` // broker name; empty = all brokers
}

type PoolConfig struct {
	MaxIdleConns      int    `json:"max_idle_conns" config:",default=200"`
	MaxIdlePerHost    int    `json:"max_idle_conns_per_host" config:",default=100"`
	MaxConnsPerHost   int    `json:"max_conns_per_host" config:",default=250"`
	IdleTimeout       string `json:"idle_timeout" config:",default=90s"`
}

type CacheConfig struct {
	L1     string `json:"l1" config:",default=ram"` // ram | none
	L1TTL  string `json:"l1_ttl" config:",default=5m"`
	L1Size int    `json:"l1_size" config:",default=10000"`
	L2     string `json:"l2" config:",optional"`     // disk | none
	L2Path string `json:"l2_path" config:",optional"`
}

type StorageDef struct {
	Mode       string       `json:"mode"` // s3, local
	Bucket     string       `json:"bucket" config:",optional"`
	Path       string       `json:"path" config:",optional"`
	Region     string       `json:"region" config:",optional"`
	Endpoint   string       `json:"endpoint" config:",optional"`
	AccessKey  string       `json:"access_key" config:",optional"`
	SecretKey  string       `json:"secret_key" config:",optional"`
	Presign    bool         `json:"presign" config:",optional"`
	PresignTTL string       `json:"presign_ttl" config:",default=5m"`
	Pool       *PoolConfig  `json:"pool" config:",optional"`
	Cache      *CacheConfig `json:"cache" config:",optional"`
}

func (e *EntryDef) Validate() error {
	if e.Type == "" {
		return fmt.Errorf("type is required")
	}
	switch e.Type {
	case "crud":
		return e.validateCRUD()
	case "rest":
		return e.validateREST()
	case "webhook":
		return e.validateWebhook()
	case "websocket":
		return e.validateWebSocket()
	case "sse":
		return e.validateSSE()
	case "file":
		return e.validateFile()
	case "async":
		return e.validateAsync()
	case "graphql":
		return e.validateGraphQL()
	default:
		return fmt.Errorf("unknown entry type %q (use crud, rest, webhook, websocket, sse, file, async, or graphql)", e.Type)
	}
}

func (e *EntryDef) validateCRUD() error {
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
}

func (e *EntryDef) validateREST() error {
	if e.Method == "" {
		return fmt.Errorf("rest: method is required")
	}
	if e.Path == "" {
		return fmt.Errorf("rest: path is required")
	}
	if e.Handler == "" {
		return fmt.Errorf("rest: handler is required")
	}
	return nil
}

func (e *EntryDef) validateWebhook() error {
	if e.Method == "" {
		e.Method = "POST"
	}
	if e.Path == "" {
		return fmt.Errorf("webhook: path is required")
	}
	if e.Handler == "" {
		return fmt.Errorf("webhook: handler is required")
	}
	return nil
}

func (e *EntryDef) validateWebSocket() error {
	if e.Path == "" {
		return fmt.Errorf("websocket: path is required")
	}
	if e.Handler == "" {
		return fmt.Errorf("websocket: handler is required")
	}
	return nil
}

func (e *EntryDef) validateSSE() error {
	if e.Path == "" {
		return fmt.Errorf("sse: path is required")
	}
	if e.Handler == "" {
		return fmt.Errorf("sse: handler is required")
	}
	return nil
}

func (e *EntryDef) validateFile() error {
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
	return nil
}

func (e *EntryDef) validateAsync() error {
	if e.Path == "" {
		return fmt.Errorf("async: path is required")
	}
	if e.Handler == "" {
		return fmt.Errorf("async: handler is required")
	}
	return nil
}

func (e *EntryDef) validateGraphQL() error {
	if e.Path == "" {
		return fmt.Errorf("graphql: path is required")
	}
	return nil
}

// ---- Exit Workers ----

type ExitWorker struct {
	Name          string       `json:"name"`
	Subscribe     SubscribeDef `json:"subscribe"`
	Handler       string       `json:"handler"`
	MaxConcurrent int          `json:"max_concurrent" config:",default=1"`
	DB            string       `json:"db" config:",optional"`
	Reply         bool         `json:"reply" config:",optional"`
	ReplyTimeout  string       `json:"reply_timeout" config:",default=30s"`
	PullBatch     int          `json:"pull_batch" config:",optional"`
	PullMaxWait   string       `json:"pull_max_wait" config:",optional"`
	ConsumerMode  string       `json:"consumer_mode" config:",optional"` // push or pull
	EventStream   string       `json:"event_stream" config:",optional"`  // broker name
}

type SubscribeDef struct {
	Stream  string `json:"stream"`
	Subject string `json:"subject" config:",optional"`
	Durable string `json:"durable" config:",optional"`
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
	Mode     string       `json:"mode" config:",default=nats"` // nats, handler, internal
	Publish  *CronPublish `json:"publish" config:",optional"`
	Handler  string       `json:"handler" config:",optional"`
}

type CronPublish struct {
	Stream  string `json:"stream"`
	Subject string `json:"subject" config:",optional"`
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

// ---- SOPS decryption ----

func trySOPSDecrypt(data []byte) ([]byte, error) {
	// Check if file is SOPS-encrypted (has "sops:" envelope or .enc.yaml path)
	if !bytes.Contains(data, []byte("sops:")) && !bytes.Contains(data, []byte("encrypted_regex")) {
		return data, nil
	}
	if _, err := exec.LookPath("sops"); err != nil {
		logx.Errorf("config: file appears SOPS-encrypted but 'sops' binary not found in PATH")
		return data, nil
	}
	cmd := exec.CommandContext(context.Background(), "sops", "--decrypt", "--input-type", "yaml", "/dev/stdin")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("sops decrypt: %w", err)
	}
	return out, nil
}

// ---- LoadConfig ----

func expandEnvDefaults(content string) (string, error) {
	var missing []string
	var result strings.Builder
	i := 0
	for i < len(content) {
		if content[i] == '$' && i+2 < len(content) && content[i+1] == '{' {
			end := strings.Index(content[i:], "}")
			if end < 0 {
				result.WriteString(string(content[i]))
				i++
				continue
			}
			expr := content[i+2 : i+end]
			parts := strings.SplitN(expr, ":", 2)
			envName := parts[0]
			envVal := os.Getenv(envName)
			switch {
			case envVal != "":
				result.WriteString(envVal)
			case len(parts) > 1:
				result.WriteString(parts[1])
			default:
				missing = append(missing, envName)
				result.WriteString("${" + envName + "}")
			}
			i += end + 1
		} else {
			result.WriteString(string(content[i]))
			i++
		}
	}
	if len(missing) > 0 {
		return result.String(), fmt.Errorf("required env vars not set: %s", strings.Join(missing, ", "))
	}
	return result.String(), nil
}

func LoadConfig(path string) (*ServiceConfig, error) {
	if path == "" {
		return nil, nil
	}
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return ParseConfig(content)
}

func ParseConfig(content []byte) (*ServiceConfig, error) {
	var cfg ServiceConfig
	decrypted, err := trySOPSDecrypt(content)
	if err != nil {
		return nil, fmt.Errorf("decrypt config: %w", err)
	}
	expanded, err := expandEnvDefaults(string(decrypted))
	if err != nil {
		return nil, err
	}
	if err := conf.LoadFromYamlBytes([]byte(expanded), &cfg); err != nil {
		return nil, err
	}
	applyEnvOverrides(&cfg)
	if err := validateConfigDeploy(&cfg); err != nil {
		return nil, err
	}
	if err := validateConfigDatabases(&cfg); err != nil {
		return nil, err
	}
	if err := validateConfigEventStreams(&cfg); err != nil {
		return nil, err
	}
	if err := validateConfigEntries(&cfg); err != nil {
		return nil, err
	}
	if err := validateConfigExits(&cfg); err != nil {
		return nil, err
	}
	if err := validateConfigCron(&cfg); err != nil {
		return nil, err
	}
	checkPlaintextSecrets(&cfg)
	return &cfg, nil
}

func applyEnvOverrides(cfg *ServiceConfig) {
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.Port = p
		}
	}
}

func validateConfigDatabases(cfg *ServiceConfig) error {
	seen := make(map[string]bool)
	for i := range cfg.Databases {
		if err := cfg.Databases[i].Validate(); err != nil {
			return fmt.Errorf("databases[%d] (%s): %w", i, cfg.Databases[i].Name, err)
		}
		if seen[cfg.Databases[i].Name] {
			return fmt.Errorf("databases[%d]: duplicate name %q", i, cfg.Databases[i].Name)
		}
		seen[cfg.Databases[i].Name] = true
	}
	return nil
}

func validateConfigEventStreams(cfg *ServiceConfig) error {
	seen := make(map[string]bool)
	for i := range cfg.EventStreams {
		if err := cfg.EventStreams[i].Validate(); err != nil {
			return fmt.Errorf("event_streams[%d] (%s): %w", i, cfg.EventStreams[i].Name, err)
		}
		if seen[cfg.EventStreams[i].Name] {
			return fmt.Errorf("event_streams[%d]: duplicate name %q", i, cfg.EventStreams[i].Name)
		}
		seen[cfg.EventStreams[i].Name] = true
	}
	return nil
}

func validateConfigEntries(cfg *ServiceConfig) error {
	seenDB := make(map[string]bool)
	for i := range cfg.Databases {
		seenDB[cfg.Databases[i].Name] = true
	}
	for i := range cfg.Entry {
		if err := cfg.Entry[i].Validate(); err != nil {
			return fmt.Errorf("entry[%d] (%s %s): %w", i, cfg.Entry[i].Type, cfg.Entry[i].Path, err)
		}
		if cfg.Entry[i].DB != "" && !seenDB[cfg.Entry[i].DB] && len(cfg.Databases) > 0 {
			return fmt.Errorf("entry[%d] (%s): db %q not found in databases", i, cfg.Entry[i].Path, cfg.Entry[i].DB)
		}
	}
	return nil
}

func validateConfigExits(cfg *ServiceConfig) error {
	seenDB := make(map[string]bool)
	for i := range cfg.Databases {
		seenDB[cfg.Databases[i].Name] = true
	}
	for i := range cfg.Exit {
		if err := cfg.Exit[i].Validate(); err != nil {
			return fmt.Errorf("exit[%d] (%s): %w", i, cfg.Exit[i].Name, err)
		}
		if cfg.Exit[i].DB != "" && !seenDB[cfg.Exit[i].DB] && len(cfg.Databases) > 0 {
			return fmt.Errorf("exit[%d] (%s): db %q not found in databases", i, cfg.Exit[i].Name, cfg.Exit[i].DB)
		}
	}
	return nil
}

func validateConfigCron(cfg *ServiceConfig) error {
	for i := range cfg.Cron {
		if err := cfg.Cron[i].Validate(); err != nil {
			return fmt.Errorf("cron[%d] (%s): %w", i, cfg.Cron[i].Name, err)
		}
	}
	return nil
}

func checkPlaintextSecrets(cfg *ServiceConfig) {
	// Check database URLs
	for i, db := range cfg.Databases {
		if looksLikePlaintextSecret(db.URL) {
			logx.Errorf("config: databases[%d].url appears to contain a plaintext secret (use ${VAR} instead)", i)
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
	"ID":   "id",
	"URL":  "url",
	"API":  "api",
	"JSON": "json",
	"XML":  "xml",
	"HTML": "html",
	"SQL":  "sql",
	"SSH":  "ssh",
	"UUID": "uuid",
	"JWT":  "jwt",
	"NATS": "nats",
	"HTTP": "http",
	"DB":   "db",
	"WS":   "ws",
	"SSE":  "sse",
}

func toSnake(s string) string {
	if mapped, ok := specialToSnake[s]; ok {
		return mapped
	}
	if len(s) <= 1 {
		return s
	}
	result := make([]byte, 0, len(s)+4)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shouldInsertUnderscore(s, i, c) {
			result = append(result, '_')
		}
		if c >= 'A' && c <= 'Z' {
			result = append(result, c+32)
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

func shouldInsertUnderscore(s string, i int, c byte) bool {
	if i == 0 || c < 'A' || c > 'Z' {
		return false
	}
	prev := s[i-1]
	var next byte
	if i+1 < len(s) {
		next = s[i+1]
	}
	return (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z' && next >= 'a' && next <= 'z')
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
