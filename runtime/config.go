package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/natuleadan/sdk-api/infra/conf"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// ---- Top-level ----

type DeployConfig struct {
	// Target is a constant.
	Target string `json:"target" config:",default=auto"`
}

type ServiceConfig struct {
	// Name is a constant.
	Name string `json:"name"`
	// Port is a constant.
	Port int `json:"port" config:",default=8080"`
	// Deploy is a constant.
	Deploy *DeployConfig `json:"deploy" config:",optional"`
	// Server is a constant.
	Server ServerConf `json:"server" config:",optional"`
	// Databases is a constant.
	Databases []DBConfig `json:"databases" config:",optional"`
	// KV is a constant.
	KV []KVConfig `json:"kv" config:",optional"`
	// Stream is a constant.
	Stream []StreamConfig `json:"stream" config:",optional"`
	// Entry is a constant.
	Entry []EntryDef `json:"entry" config:",optional"`
	// Exit is a constant.
	Exit []ExitWorker `json:"exit" config:",optional"`
	// Cron is a constant.
	Cron []CronJob `json:"cron" config:",optional"`
	// Auth is a constant.
	Auth *AuthConfig `json:"auth" config:",optional"`
	// Log is a constant.
	Log *logx.LogConf `json:"log" config:",optional"`
}

type AuthConfig struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",optional"`
	// Driver is a constant.
	Driver string `json:"driver" config:",default=none"` // none | manual | openfga-zitadel | ory
	// Secret is a constant.
	Secret string `json:"secret" config:",optional"`
	// PrevSecret is a constant.
	PrevSecret string `json:"prev_secret" config:",optional"`
	// Algorithm is a constant.
	Algorithm string `json:"algorithm" config:",default=HS256"`
	// ContextKey is a constant.
	ContextKey string `json:"context_key" config:",default=claims"`
	// Issuer is a constant.
	Issuer string `json:"issuer" config:",optional"`
	// Audience is a constant.
	Audience string `json:"audience" config:",optional"`
	// Expiry is a constant.
	Expiry int `json:"expiry" config:",default=900"` // JWT TTL in seconds (default 15 min)
	// ZitadelURL is a constant.
	ZitadelURL string `json:"zitadel_url" config:",optional"`
	// OpenFGAURL is a constant.
	OpenFGAURL string `json:"openfga_url" config:",optional"`
	// OpenFGAStore is a constant.
	OpenFGAStore string `json:"openfga_store" config:",optional"`
	// KratosURL is a constant.
	KratosURL string `json:"kratos_url" config:",optional"`
	// KetoURL is a constant.
	KetoURL string `json:"keto_url" config:",optional"`
	// Refresh is a constant.
	Refresh *RefreshConfig `json:"refresh" config:",optional"`
	// Cookie is a constant.
	Cookie *AuthCookieConfig `json:"cookie" config:",optional"`
}

type RefreshConfig struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",default=false"`
	// TTL is a constant.
	TTL int `json:"ttl" config:",default=604800"` // 7 days in seconds
	// Endpoint is a constant.
	Endpoint string `json:"endpoint" config:",default=/auth/refresh"`
	// Secret is a constant.
	Secret string `json:"secret" config:",optional"` // separate from auth.secret
	// ZitadelTokenURL is a constant.
	ZitadelTokenURL string `json:"zitadel_token_url" config:",optional"`
	// ZitadelClientID is a constant.
	ZitadelClientID string `json:"zitadel_client_id" config:",optional"`
	// KratosRefreshURL is a constant.
	KratosRefreshURL string `json:"kratos_refresh_url" config:",optional"`
}

type AuthCookieConfig struct {
	// AccessTokenName is a constant.
	AccessTokenName string `json:"access_token_name" config:",default=token"`
	// RefreshTokenName is a constant.
	RefreshTokenName string `json:"refresh_token_name" config:",default=refresh_token"`
	// Domain is a constant.
	Domain string `json:"domain" config:",optional"`
	// Path is a constant.
	Path string `json:"path" config:",default=/"`
	// HTTPOnly is a constant.
	HTTPOnly bool `json:"http_only" config:",default=true"`
	// Secure is a constant.
	Secure bool `json:"secure" config:",default=true"`
	// SameSite is a constant.
	SameSite string `json:"same_site" config:",default=Strict"`
}

// ---- Server ----

type ServerConf struct {
	// Host is a constant.
	Host string `json:"host" config:",default=0.0.0.0"`
	// Prefork is a constant.
	Prefork bool `json:"prefork" config:",optional"`
	// BodyLimit is a constant.
	BodyLimit int `json:"body_limit" config:",default=4194304"`
	// Timeout is a constant.
	Timeout string `json:"timeout" config:",default=30s"`
	// MaxConns is a constant.
	MaxConns int `json:"max_conns" config:",default=1000"`
	// MaxBytes is a constant.
	MaxBytes int `json:"max_bytes" config:",default=4194304"`
	// MetricsPath is a constant.
	MetricsPath string `json:"metrics_path" config:",default=/metrics"`
	// HealthPath is a constant.
	HealthPath string `json:"health_path" config:",default=/health"`
	// ShutdownTimeout is a constant.
	ShutdownTimeout string `json:"shutdown_timeout" config:",default=10s"`
	// RecoverStack is a constant.
	RecoverStack bool `json:"recover_stack" config:",default=true"`
	// APIPrefix is a constant.
	APIPrefix string `json:"api_prefix" config:",default=/api"`
	// CORS is a constant.
	CORS *CORSConf `json:"cors" config:",optional"`
	// Middleware is a constant.
	Middleware []RouteMW `json:"middleware" config:",optional"`
	// Static is a constant.
	Static []StaticDef `json:"static" config:",optional"`
	// OpenAPI is a constant.
	OpenAPI *OpenAPIConf `json:"openapi" config:",optional"`
	// SecurityHeaders is a constant.
	SecurityHeaders *SecurityHeadersConf `json:"security_headers" config:",optional"`
	// CSRF is a constant.
	CSRF *CSRFConf `json:"csrf" config:",optional"`
	// RateLimit is a constant.
	RateLimit *RateLimitConf `json:"rate_limit" config:",optional"`
	// TLS is a constant.
	TLS *TLSConf `json:"tls" config:",optional"`
	// SSRF is a constant.
	SSRF *SSRFConf `json:"ssrf" config:",optional"`
	// Cookies is a constant.
	Cookies *CookieConf `json:"cookies" config:",optional"`
	// Security is a constant.
	Security *SecurityDef `json:"security" config:",optional"`
	// SlowQueryThreshold is a constant.
	SlowQueryThreshold string `json:"slow_query_threshold" config:",default=100ms"`
	// Logger is a constant.
	Logger bool `json:"logger" config:",default=true"`
	// LoadShedding is a constant.
	LoadShedding bool `json:"load_shedding" config:",default=true"`
	// Breaker is a constant.
	Breaker bool `json:"breaker" config:",default=true"`
	// Telemetry is a constant.
	Telemetry *TelemetryConf `json:"telemetry" config:",optional"`
	// Correlation enables the X-Correlation-ID tracking middleware.
	Correlation *CorrelationConf `json:"correlation" config:",optional"`
	// GrpcServer configures the gRPC server.
	GrpcServer *GrpcServerConf `json:"grpc_server" config:",optional"`
	// GrpcClients defines gRPC client connections to other services.
	GrpcClients []GrpcClientConf `json:"grpc_clients" config:",optional"`
}

type GrpcServerConf struct {
	// ListenOn is the address to listen on (e.g. ":8081").
	ListenOn string `json:"listen_on" config:",optional"`
	// Timeout is the default RPC timeout in milliseconds.
	Timeout int64 `json:"timeout" config:",default=2000"`
	// CpuThreshold is the CPU load threshold for adaptive shedding (0-1000). 0 disables.
	CpuThreshold int64 `json:"cpu_threshold" config:",default=900"`
	// Health enables the gRPC health check service.
	Health bool `json:"health" config:",default=true"`
}

type GrpcClientConf struct {
	// Name is a unique name for this client connection.
	Name string `json:"name"`
	// Target is the gRPC target address (e.g. "dns:///product-svc:8081").
	Target string `json:"target" config:",optional"`
	// Endpoints are direct gRPC endpoints (e.g. ["localhost:8081"]).
	Endpoints []string `json:"endpoints" config:",optional"`
	// Timeout is the default RPC timeout in milliseconds.
	Timeout int64 `json:"timeout" config:",default=2000"`
	// NonBlock enables non-blocking dial.
	NonBlock bool `json:"non_block" config:",default=true"`
}

type CorrelationConf struct {
	// Enabled enables the correlation ID middleware.
	Enabled bool `json:"enabled" config:",optional"`
	// RequestHeader is the header to read the correlation ID from.
	RequestHeader string `json:"request_header" config:",default=X-Correlation-ID"`
	// ResponseHeader is the header to set the correlation ID on.
	ResponseHeader string `json:"response_header" config:",default=X-Correlation-ID"`
	// SkipPaths are request paths that should not receive a correlation ID.
	SkipPaths []string `json:"skip_paths" config:",optional"`
}

type TelemetryConf struct {
	Enabled bool   `json:"enabled" config:",optional"`
	Name    string `json:"name" config:",optional"`
	// Endpoint is the OTLP receiver address (e.g. "localhost:4317").
	Endpoint string  `json:"endpoint" config:",optional"`
	Sampler  float64 `json:"sampler" config:",default=1.0"`
	// Batcher is the exporter type: otlpgrpc | otlphttp | zipkin | file.
	Batcher string `json:"batcher" config:",default=otlpgrpc"`
	// OtlpHeaders are additional headers sent with OTLP export requests.
	OtlpHeaders map[string]string `json:"otlp_headers" config:",optional"`
	// OtlpHttpPath is the URL path for OTLP HTTP transport (e.g. "/v1/traces").
	OtlpHttpPath string `json:"otlp_http_path" config:",optional"`
	// OtlpHttpSecure enables TLS for OTLP HTTP transport.
	OtlpHttpSecure bool `json:"otlp_http_secure" config:",optional"`
	// TraceResponseHeader sets a response header exposing the trace ID (e.g. "X-Trace-Id").
	TraceResponseHeader string `json:"trace_response_header" config:",optional"`
	// SkipPaths are request paths that should not be traced.
	SkipPaths []string `json:"skip_paths" config:",optional"`
}

type SecurityDef struct {
	// ContentSecurity is a constant.
	ContentSecurity *ContentSecurityDef `json:"content_security" config:",optional"`
	// Cryption is a constant.
	Cryption *CryptionDef `json:"cryption" config:",optional"`
	// EncryptCookie is a constant.
	EncryptCookie *EncryptCookieDef `json:"encrypt_cookie" config:",optional"`
}

type ContentSecurityDef struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",optional"`
	// Strict is a constant.
	Strict bool `json:"strict" config:",optional"`
	// PublicKey is a constant.
	PublicKey string `json:"public_key"`
}

type CryptionDef struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",optional"`
	// Key is a constant.
	Key string `json:"key"`
}

type EncryptCookieDef struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",optional"`
	// Key is a constant.
	Key string `json:"key"` // required when enabled
	// Except is a constant.
	Except []string `json:"except" config:",optional"` // cookie names to skip
}

type CookieConf struct {
	// SameSite is a constant.
	SameSite string `json:"same_site" config:",optional"` // Strict, Lax, None
	// Secure is a constant.
	Secure bool `json:"secure" config:",optional"`
}

type KVConfig struct {
	// Name is a constant.
	Name string `json:"name"`
	// Driver is a constant.
	Driver string `json:"driver" config:",default=redis"`
	// URL is a constant.
	URL string `json:"url"`
}

type StreamConfig struct {
	// Name is a constant.
	Name string `json:"name"`
	// Driver is a constant.
	Driver string `json:"driver" config:",default=nats"`
	// URL is a constant.
	URL string `json:"url" config:",optional"`
	// Brokers is a constant.
	Brokers []string `json:"brokers" config:",optional"`
	// ConsumerGroup is a constant.
	ConsumerGroup string `json:"consumer_group" config:",optional"`
	// MaxReconnects is a constant.
	MaxReconnects int `json:"max_reconnects" config:",optional"`
	// ReconnectWait is a constant.
	ReconnectWait string `json:"reconnect_wait" config:",optional"`
	// Timeout is a constant.
	Timeout string `json:"timeout" config:",optional"`
	// RetryOnFail is a constant.
	RetryOnFail bool `json:"retry_on_fail" config:",optional"`
	// Streams is a constant.
	Streams []StreamDef `json:"streams" config:",optional"`
}

type RetryConf struct {
	// MaxRetries is the maximum number of retry attempts (default 3).
	MaxRetries int `json:"max_retries" config:",default=3"`
	// InitialInterval is the initial backoff duration (default 500ms).
	InitialInterval string `json:"initial_interval" config:",default=500ms"`
	// MaxBackoff is the maximum backoff duration (default 10s).
	MaxBackoff string `json:"max_backoff" config:",default=10s"`
	// Multiplier is the exponential backoff multiplier (default 2.0).
	Multiplier float64 `json:"multiplier" config:",default=2.0"`
}

type RateLimitConf struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",optional"`
	// KV is a constant.
	KV string `json:"kv" config:",optional"` // references kv[].name
	// Algorithm is a constant.
	Algorithm string `json:"algorithm" config:",default=sliding_window"`
	// TTL is a constant.
	TTL string `json:"ttl" config:",optional"`
	// Global is a constant.
	Global *RateLimitDef `json:"global" config:",optional"`
	// PerIP is a constant.
	PerIP *RateLimitDef `json:"per_ip" config:",optional"`
	// PerUser is a constant.
	PerUser *RateLimitDef `json:"per_user" config:",optional"`
	// PerKey is a constant.
	PerKey *RateLimitDef `json:"per_key" config:",optional"`
	// SkipFailedRequests is a constant.
	SkipFailedRequests bool `json:"skip_failed_requests" config:",optional"`
	// SkipSuccessfulRequests is a constant.
	SkipSuccessfulRequests bool `json:"skip_successful_requests" config:",optional"`
}

type RateLimitDef struct {
	// RequestsPerSecond is a constant.
	RequestsPerSecond int `json:"requests_per_second"`
	// Burst is a constant.
	Burst int `json:"burst"`
	// TTL is a constant.
	TTL string `json:"ttl" config:",optional"`
}

type SSRFConf struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",optional"`
	// BlockPrivate is a constant.
	BlockPrivate bool `json:"block_private" config:",optional"`
	// BlockLoopback is a constant.
	BlockLoopback bool `json:"block_loopback" config:",optional"`
	// BlockMetadata is a constant.
	BlockMetadata bool `json:"block_metadata" config:",optional"`
	// AllowedHosts is a constant.
	AllowedHosts []string `json:"allowed_hosts" config:",optional"`
}

type TLSConf struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled"`
	// Manual is a constant.
	Manual *ManualTLS `json:"manual" config:",optional"`
	// Autocert is a constant.
	Autocert *AutocertTLS `json:"autocert" config:",optional"`
	// MinVersion is a constant.
	MinVersion string `json:"min_version" config:",optional"`
	// MaxVersion is a constant.
	MaxVersion string `json:"max_version" config:",optional"`
	// CurvePrefs is a constant.
	CurvePrefs []string `json:"curve_preferences" config:",optional"`
	// CipherSuites is a constant.
	CipherSuites []string `json:"cipher_suites" config:",optional"`
	// RedirectHTTP is a constant.
	RedirectHTTP bool `json:"redirect_http" config:",optional"`
	// RedirectPort is a constant.
	RedirectPort int `json:"redirect_port" config:",optional"`
}

type ManualTLS struct {
	// CertFile is a constant.
	CertFile string `json:"cert_file"`
	// KeyFile is a constant.
	KeyFile string `json:"key_file"`
}

type AutocertTLS struct {
	// Domains is a constant.
	Domains []string `json:"domains"`
	// Email is a constant.
	Email string `json:"email"`
	// CacheDir is a constant.
	CacheDir string `json:"cache_dir" config:",optional"`
}

type SecurityHeadersConf struct {
	// FrameOptions is a constant.
	FrameOptions string `json:"frame_options" config:",optional"`
	// ReferrerPolicy is a constant.
	ReferrerPolicy string `json:"referrer_policy" config:",optional"`
	// PermissionsPolicy is a constant.
	PermissionsPolicy string `json:"permissions_policy" config:",optional"`
	// HSTS is a constant.
	HSTS bool `json:"hsts" config:",optional"`
	// HSTSMaxAge is a constant.
	HSTSMaxAge int `json:"hsts_max_age" config:",optional"`
	// HSTSIncludeSubs is a constant.
	HSTSIncludeSubs bool `json:"hsts_include_subdomains" config:",optional"`
	// CSP is a constant.
	CSP string `json:"csp" config:",optional"`
	// CSPConfig is a constant.
	CSPConfig *CSPConf `json:"csp_config" config:",optional"` // programmatic CSP builder
	// COOP is a constant.
	COOP string `json:"coop" config:",optional"`
	// COEP is a constant.
	COEP string `json:"coep" config:",optional"`
	// CORP is a constant.
	CORP string `json:"corp" config:",optional"`
	// CacheControl is a constant.
	CacheControl string `json:"cache_control" config:",optional"`
	// CSPReportPath is a constant.
	CSPReportPath string `json:"csp_report_path" config:",optional"`
}

type CSPConf struct {
	// Level is a constant.
	Level string `json:"level" config:",default=basic"`
	// DefaultSrc is a constant.
	DefaultSrc []string `json:"default_src" config:",optional"`
	// ScriptSrc is a constant.
	ScriptSrc []string `json:"script_src" config:",optional"`
	// StyleSrc is a constant.
	StyleSrc []string `json:"style_src" config:",optional"`
	// ImgSrc is a constant.
	ImgSrc []string `json:"img_src" config:",optional"`
	// ConnectSrc is a constant.
	ConnectSrc []string `json:"connect_src" config:",optional"`
	// FontSrc is a constant.
	FontSrc []string `json:"font_src" config:",optional"`
	// FrameSrc is a constant.
	FrameSrc []string `json:"frame_src" config:",optional"`
	// FrameAncestors is a constant.
	FrameAncestors []string `json:"frame_ancestors" config:",optional"`
	// ObjectSrc is a constant.
	ObjectSrc []string `json:"object_src" config:",optional"`
	// BaseURI is a constant.
	BaseURI []string `json:"base_uri" config:",optional"`
	// FormAction is a constant.
	FormAction []string `json:"form_action" config:",optional"`
	// UpgradeInsecureReq is a constant.
	UpgradeInsecureReq bool `json:"upgrade_insecure_requests" config:",optional"`
}

type CSRFConf struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",optional"`
	// CookieName is a constant.
	CookieName string `json:"cookie_name" config:",optional"`
	// HeaderName is a constant.
	HeaderName string `json:"header_name" config:",optional"`
	// SameSite is a constant.
	SameSite string `json:"same_site" config:",optional"`
	// Secure is a constant.
	Secure bool `json:"secure" config:",optional"`
	// ExcludePaths is a constant.
	ExcludePaths []string `json:"exclude_paths" config:",optional"`
	// JSONCheck is a constant.
	JSONCheck bool `json:"json_check" config:",optional"`
}

type RouteMW struct {
	// Path is a constant.
	Path string `json:"path"`
	// Apply is a constant.
	Apply []string `json:"apply"`
}

type CORSConf struct {
	// Origins is a constant.
	Origins []string `json:"origins" config:",optional"`
	// Methods is a constant.
	Methods []string `json:"methods" config:",optional"`
	// Headers is a constant.
	Headers []string `json:"headers" config:",optional"`
	// Credentials is a constant.
	Credentials bool `json:"credentials" config:",optional"`
	// MaxAge is a constant.
	MaxAge int `json:"max_age" config:",default=300"`
}

type StaticDef struct {
	// Prefix is a constant.
	Prefix string `json:"prefix"`
	// Dir is a constant.
	Dir string `json:"dir"`
}

type OpenAPIConf struct {
	// Enabled is a constant.
	Enabled bool `json:"enabled" config:",optional"`
	// Version is a constant.
	Version string `json:"version" config:",default=1.0.0"`
	// SpecPath is a constant.
	SpecPath string `json:"spec_path" config:",default=/openapi.json"`
	// DocsPath is a constant.
	DocsPath string `json:"docs_path" config:",default=/docs"`
	// Theme is a constant.
	Theme string `json:"theme" config:",default=moon"`
	// DarkMode is a constant.
	DarkMode bool `json:"dark_mode" config:",default=true"`
}

// ---- Database ----

type SlowQueryConf struct {
	// Enabled enables slow query logging for this database.
	Enabled bool `json:"enabled" config:",optional"`
	// Threshold is the duration after which a query is considered slow (e.g. "200ms", "1s").
	Threshold string `json:"threshold" config:",default=100ms"`
}

type DBConfig struct {
	// Name is a constant.
	Name string `json:"name"`
	// Driver is a constant.
	Driver string `json:"driver" config:",default=postgres"`
	// URL is a constant.
	URL string `json:"url"`
	// Database is a constant.
	Database string `json:"database" config:",optional"`
	// Pool is a constant.
	Pool *PoolConf `json:"pool" config:",optional"`
	// Turso is a constant.
	Turso *TursoConf `json:"turso" config:",optional"`
	// SlowQuery is a constant.
	SlowQuery *SlowQueryConf `json:"slow_query" config:",optional"`
}

type TursoConf struct {
	// Mode is a constant.
	Mode string `json:"mode" config:",default=local"` // local | remote
	// BusyTimeout is a constant.
	BusyTimeout int `json:"busy_timeout" config:",default=30000"` // ms, 0 = no wait
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
	// MaxConns is a constant.
	MaxConns int32 `json:"max_conns" config:",default=10"`
	// MinConns is a constant.
	MinConns int32 `json:"min_conns" config:",default=2"`
	// MaxConnLifetime is a constant.
	MaxConnLifetime string `json:"max_conn_lifetime" config:",optional"`
	// MaxConnIdleTime is a constant.
	MaxConnIdleTime string `json:"max_conn_idle_time" config:",optional"`
	// HealthCheckPeriod is a constant.
	HealthCheckPeriod string `json:"health_check_period" config:",optional"`
	// ReservedConns is a constant.
	ReservedConns int32 `json:"reserved_conns" config:",default=10"`
}

func (d *DBConfig) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if d.URL == "" {
		return fmt.Errorf("url is required")
	}
	if d.Driver == "" {
		return fmt.Errorf("driver is required (postgres, mysql, turso, or mongo)")
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
	// Name is a constant.
	Name string `json:"name"`
	// Driver is a constant.
	Driver string `json:"driver"` // nats, kafka
	// URL is a constant.
	URL string `json:"url" config:",optional"`
	// Brokers is a constant.
	Brokers []string `json:"brokers" config:",optional"`
	// ConsumerGroup is a constant.
	ConsumerGroup string `json:"consumer_group" config:",optional"`
	// MaxReconnects is a constant.
	MaxReconnects int `json:"max_reconnects" config:",optional"`
	// ReconnectWait is a constant.
	ReconnectWait string `json:"reconnect_wait" config:",optional"`
	// Timeout is a constant.
	Timeout string `json:"timeout" config:",optional"`
	// RetryOnFail is a constant.
	RetryOnFail bool `json:"retry_on_fail" config:",optional"`
	// Streams is a constant.
	Streams []StreamDef `json:"streams" config:",optional"`
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
	// Name is a constant.
	Name string `json:"name"`
	// MaxAge is a constant.
	MaxAge string `json:"max_age" config:",optional"`
	// MaxBytes is a constant.
	MaxBytes int64 `json:"max_bytes" config:",optional"`
	// Storage is a constant.
	Storage string `json:"storage" config:",default=file"`
	// Compression is a constant.
	Compression string `json:"compression" config:",default=s2"`
}

// ---- Entry Endpoints ----

type EntryDef struct {
	// Type is a constant.
	Type string `json:"type"` // crud, rest, webhook, websocket, sse, file
	// Method is a constant.
	Method string `json:"method" config:",optional"`
	// Path is a constant.
	Path string `json:"path" config:",optional"`
	// Handler is a constant.
	Handler string `json:"handler" config:",optional"`
	// AuthModes is a constant.
	AuthModes []string `json:"auth_modes" config:",optional"` // ["jwt"], ["apikey"], ["jwt","apikey"]
	// JWTFrom is a constant.
	JWTFrom string `json:"jwt_from" config:",optional"` // per-entry: "header:Authorization", "cookie:token", "query:token"
	// Roles is a constant.
	Roles []string `json:"roles" config:",optional"`
	// Permissions is a constant.
	Permissions []string `json:"permissions" config:",optional"`
	// DB is a constant.
	DB string `json:"db" config:",optional"` // references database name
	// TenantScope is a constant.
	TenantScope string `json:"tenant_scope" config:",optional"` // JWT claim for tenant ID (e.g. "org_id")
	// TenantField is a constant.
	TenantField string `json:"tenant_field" config:",optional"` // DB column for tenant filter (e.g. "tenant_id")

	// ServiceName is the gRPC service name (required for type: grpc).
	ServiceName string `json:"service_name" config:",optional"`

	// CRUD
	Model string `json:"model" config:",optional"`
	// Table is a constant.
	Table string `json:"table" config:",optional"`
	// Resource is a constant.
	Resource string `json:"resource" config:",optional"`
	// Overrides is a constant.
	Overrides *CRUDOverrides `json:"overrides" config:",optional"`

	// Event stream selection
	EventStream string `json:"event_stream" config:",optional"`

	// Event publish targets
	EventPublish []EventPublishTarget `json:"event_publish" config:",optional"`

	// File
	AllowedTypes []string `json:"allowed_types" config:",optional"`
	// MaxSize is a constant.
	MaxSize string `json:"max_size" config:",optional"`
	// MaxFiles is a constant.
	MaxFiles int `json:"max_files" config:",optional"`
	// MagicBytes is a constant.
	MagicBytes bool `json:"magic_bytes" config:",optional"`
	// Storage is a constant.
	Storage *StorageDef `json:"storage" config:",optional"`

	// Security per-entry overrides
	CSRF *bool `json:"csrf" config:",optional"` // false = skip CSRF for this entry
	// RequiresMFA is a constant.
	RequiresMFA bool `json:"requires_mfa" config:",optional"` // true = MFA must be verified
	// RateLimit is a constant.
	RateLimit *RateLimitDef `json:"rate_limit" config:",optional"` // per-entry rate limit (pre-auth)
	// RateLimitPerUser is a constant.
	RateLimitPerUser *RateLimitDef `json:"rate_limit_per_user" config:",optional"` // per-entry per-user rate limit (post-auth)
	// RateLimitPerKey is a constant.
	RateLimitPerKey *RateLimitDef `json:"rate_limit_per_key" config:",optional"` // per-entry per-key rate limit (post-auth)
	// PerRoleLimits is a constant.
	PerRoleLimits map[string]*RateLimitDef `json:"rate_limit_per_role" config:",optional"` // per-role rate limits
	// Cache is a constant.
	Cache string `json:"cache" config:",optional"` // references kv[].name for CRUD cache

	// Validation
	ValidationModel string `json:"validate" config:",optional"` // validation model name

	// APIVersion sets the API version prefix for this entry (e.g. "v1", "v2").
	// If empty and the server api_prefix does not already contain a version,
	// defaults to "v1".
	APIVersion string `json:"api_version" config:",optional"`

	// APIStatus indicates the lifecycle status of this endpoint.
	// Values: current | deprecated | removed
	APIStatus string `json:"api_status" config:",optional"`
	// SunsetDate is the RFC3339 date when the endpoint will be removed.
	SunsetDate string `json:"sunset_date" config:",optional"`

	// Timeout per-entry (e.g. "30s")
	Timeout string `json:"timeout" config:",optional"`

	// Retry configures the retry behavior for idempotent methods (GET, HEAD, PUT, DELETE, OPTIONS).
	Retry *RetryConf `json:"retry" config:",optional"`
	// Fallback sets the fallback strategy when the circuit breaker is open.
	// Values: "degraded" | "stale" | "" (disabled)
	Fallback string `json:"fallback" config:",optional"`
	// Bulkhead defines named concurrency limits for external outbound calls.
	// Each key is a dependency name, value is max concurrent calls.
	Bulkhead map[string]int `json:"bulkhead" config:",optional"`

	// API Key prefix (only applies when auth_modes includes "apikey")
	APIPrefix string `json:"api_key_prefix" config:",optional"`

	// Pagination (CRUD only)
	PageSize int `json:"page_size" config:",optional"` // default 10, also min
	// MaxPageSize is a constant.
	MaxPageSize int `json:"max_page_size" config:",optional"` // default 100, also max
	// Pagination is a constant.
	Pagination string `json:"pagination" config:",optional"` // "offset" | "keyset"
	// Sortable is a constant.
	Sortable []string `json:"sortable" config:",optional"` // allowed sort columns
}

type CRUDOverrides struct {
	// List is a constant.
	List string `json:"list" config:",optional"`
	// Get is a constant.
	Get string `json:"get" config:",optional"`
	// Create is a constant.
	Create string `json:"create" config:",optional"`
	// Update is a constant.
	Update string `json:"update" config:",optional"`
	// Delete is a constant.
	Delete string `json:"delete" config:",optional"`
}

type EventPublishTarget struct {
	// Stream is a constant.
	Stream string `json:"stream"`
	// Subject is a constant.
	Subject string `json:"subject" config:",optional"`
	// EventStream is a constant.
	EventStream string `json:"event_stream" config:",optional"` // broker name; empty = all brokers
}

// hasAuth returns true if the entry's auth_modes includes the given mode.
func hasAuth(entry *EntryDef, mode string) bool {
	return slices.Contains(entry.AuthModes, mode)
}

type PoolConfig struct {
	// MaxIdleConns is a constant.
	MaxIdleConns int `json:"max_idle_conns" config:",default=200"`
	// MaxIdlePerHost is a constant.
	MaxIdlePerHost int `json:"max_idle_conns_per_host" config:",default=100"`
	// MaxConnsPerHost is a constant.
	MaxConnsPerHost int `json:"max_conns_per_host" config:",default=250"`
	// IdleTimeout is a constant.
	IdleTimeout string `json:"idle_timeout" config:",default=90s"`
}

type CacheConfig struct {
	// L1 is a constant.
	L1 string `json:"l1" config:",default=ram"` // ram | none
	// L1TTL is a constant.
	L1TTL string `json:"l1_ttl" config:",default=5m"`
	// L1Size is a constant.
	L1Size int `json:"l1_size" config:",default=10000"`
	// L2 is a constant.
	L2 string `json:"l2" config:",optional"` // disk | none
	// L2Path is a constant.
	L2Path string `json:"l2_path" config:",optional"`
}

type StorageDef struct {
	// Mode is a constant.
	Mode string `json:"mode"` // s3, local
	// Bucket is a constant.
	Bucket string `json:"bucket" config:",optional"`
	// Path is a constant.
	Path string `json:"path" config:",optional"`
	// Region is a constant.
	Region string `json:"region" config:",optional"`
	// Endpoint is a constant.
	Endpoint string `json:"endpoint" config:",optional"`
	// AccessKey is a constant.
	AccessKey string `json:"access_key" config:",optional"`
	// SecretKey is a constant.
	SecretKey string `json:"secret_key" config:",optional"`
	// Presign is a constant.
	Presign bool `json:"presign" config:",optional"`
	// PresignTTL is a constant.
	PresignTTL string `json:"presign_ttl" config:",default=5m"`
	// Pool is a constant.
	Pool *PoolConfig `json:"pool" config:",optional"`
	// Cache is a constant.
	Cache *CacheConfig `json:"cache" config:",optional"`
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
	case "grpc":
		return e.validateGRPC()
	default:
		return fmt.Errorf("unknown entry type %q (use crud, rest, webhook, websocket, sse, file, async, grpc, or graphql)", e.Type)
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

func (e *EntryDef) validateGRPC() error {
	if e.ServiceName == "" {
		return fmt.Errorf("grpc: service name is required")
	}
	if e.Handler == "" {
		return fmt.Errorf("grpc: handler is required")
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
	// Name is a constant.
	Name string `json:"name"`
	// Subscribe is a constant.
	Subscribe SubscribeDef `json:"subscribe"`
	// Handler is a constant.
	Handler string `json:"handler"`
	// MaxConcurrent is a constant.
	MaxConcurrent int `json:"max_concurrent" config:",default=1"`
	// DB is a constant.
	DB string `json:"db" config:",optional"`
	// Reply is a constant.
	Reply bool `json:"reply" config:",optional"`
	// ReplyTimeout is a constant.
	ReplyTimeout string `json:"reply_timeout" config:",default=30s"`
	// PullBatch is a constant.
	PullBatch int `json:"pull_batch" config:",optional"`
	// PullMaxWait is a constant.
	PullMaxWait string `json:"pull_max_wait" config:",optional"`
	// ConsumerMode is a constant.
	ConsumerMode string `json:"consumer_mode" config:",optional"` // push or pull
	// EventStream is a constant.
	EventStream string `json:"event_stream" config:",optional"` // broker name
}

type SubscribeDef struct {
	// Stream is a constant.
	Stream string `json:"stream"`
	// Subject is a constant.
	Subject string `json:"subject" config:",optional"`
	// Durable is a constant.
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
	// Name is a constant.
	Name string `json:"name"`
	// Schedule is a constant.
	Schedule string `json:"schedule"`
	// Mode is a constant.
	Mode string `json:"mode" config:",default=nats"` // nats, handler, internal
	// Publish is a constant.
	Publish *CronPublish `json:"publish" config:",optional"`
	// Handler is a constant.
	Handler string `json:"handler" config:",optional"`
}

type CronPublish struct {
	// Stream is a constant.
	Stream string `json:"stream"`
	// Subject is a constant.
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

func loadDotEnv() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}

func LoadConfig(path string) (*ServiceConfig, error) {
	loadDotEnv()
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
	if err := validateConfigStream(&cfg); err != nil {
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
	if err := validateConfigKV(&cfg); err != nil {
		return nil, err
	}
	if err := validateConfigRateLimit(&cfg); err != nil {
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

func validateConfigStream(cfg *ServiceConfig) error {
	seen := make(map[string]bool)
	for i := range cfg.Stream {
		st := &cfg.Stream[i]
		if st.Name == "" {
			return fmt.Errorf("stream[%d]: name is required", i)
		}
		if seen[st.Name] {
			return fmt.Errorf("stream[%d]: duplicate name %q", i, st.Name)
		}
		seen[st.Name] = true
		if st.Driver != "nats" && st.Driver != "kafka" {
			return fmt.Errorf("stream[%d] (%s): driver must be nats or kafka", i, st.Name)
		}
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

func validateConfigKV(cfg *ServiceConfig) error {
	seen := make(map[string]bool)
	for i := range cfg.KV {
		kv := &cfg.KV[i]
		if kv.Name == "" {
			return fmt.Errorf("kv[%d]: name is required", i)
		}
		if seen[kv.Name] {
			return fmt.Errorf("kv[%d]: duplicate name %q", i, kv.Name)
		}
		seen[kv.Name] = true
		if kv.URL == "" {
			return fmt.Errorf("kv[%d] (%s): url is required", i, kv.Name)
		}
	}
	return nil
}

func validateConfigRateLimit(cfg *ServiceConfig) error {
	if cfg.Server.RateLimit == nil || !cfg.Server.RateLimit.Enabled {
		return nil
	}
	if cfg.Server.RateLimit.KV != "" {
		found := false
		for _, kv := range cfg.KV {
			if kv.Name == cfg.Server.RateLimit.KV {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("server.rate_limit.kv %q not found in kv[] section", cfg.Server.RateLimit.KV)
		}
	}
	// Also check entries referencing kv
	seenKV := make(map[string]bool)
	for _, kv := range cfg.KV {
		seenKV[kv.Name] = true
	}
	for i, entry := range cfg.Entry {
		if entry.Cache != "" && !seenKV[entry.Cache] && len(cfg.KV) > 0 {
			return fmt.Errorf("entry[%d] (%s): cache %q not found in kv[] section", i, entry.Path, entry.Cache)
		}
	}
	return nil
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
