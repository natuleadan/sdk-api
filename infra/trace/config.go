package trace

// TraceName represents the tracing name.
const TraceName = "go-zero"

// A Config is an opentelemetry config.
type Config struct {
	Name     string  `config:",optional"`
	Endpoint string  `config:",optional"`
	Sampler  float64 `config:",default=1.0"`
	Batcher  string  `config:",default=otlpgrpc,options=zipkin|otlpgrpc|otlphttp|file"`
	// OtlpHeaders represents the headers for OTLP gRPC or HTTP transport.
	// For example:
	//  uptrace-dsn: 'http://project2_secret_token@localhost:14317/2'
	OtlpHeaders map[string]string `config:",optional"`
	// OtlpHttpPath represents the path for OTLP HTTP transport.
	// For example
	// /v1/traces
	OtlpHttpPath string `config:",optional"`
	// OtlpHttpSecure represents the scheme to use for OTLP HTTP transport.
	OtlpHttpSecure bool `config:",optional"`
	// Disabled indicates whether StartAgent starts the agent.
	Disabled bool `config:",optional"`
}
