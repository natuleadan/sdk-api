package devserver

// Config is config for inner http server.
type Config struct {
	Enabled        bool   `config:",default=true"`
	Host           string `config:",optional"`
	Port           int    `config:",default=6060"`
	MetricsPath    string `config:",default=/metrics"`
	HealthPath     string `config:",default=/healthz"`
	EnableMetrics  bool   `config:",default=true"`
	EnablePprof    bool   `config:",default=true"`
	HealthResponse string `config:",default=OK"`
}
