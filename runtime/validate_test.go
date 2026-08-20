package runtime

import (
	"strings"
	"testing"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/logx/logtest"
)

// warningCollector returns a log collector with Info level forced on, so
// tests are not affected by other runtime tests calling logx.Disable().
func warningCollector(t *testing.T) *logtest.Buffer {
	t.Helper()
	logx.SetLevel(logx.InfoLevel)
	return logtest.NewCollector(t)
}

func TestCheckScalarWarnings_NoCORS_NoCSP(t *testing.T) {
	collector := warningCollector(t)
	cfg := &ServiceConfig{
		Server: ServerConf{
			OpenAPI: &OpenAPIConf{Enabled: true},
		},
	}
	CheckScalarWarnings(cfg)
	out := collector.String()
	if !strings.Contains(out, "no CORS configured") {
		t.Errorf("expected CORS warning, got: %s", out)
	}
	if !strings.Contains(out, "no CSP configured") {
		t.Errorf("expected CSP warning, got: %s", out)
	}
	if !strings.Contains(out, "102-scalar-ui") {
		t.Errorf("expected reference to examples/102-scalar-ui, got: %s", out)
	}
}

func TestCheckScalarWarnings_Disabled_NoWarnings(t *testing.T) {
	collector := warningCollector(t)
	cfg := &ServiceConfig{
		Server: ServerConf{
			OpenAPI: &OpenAPIConf{Enabled: false},
		},
	}
	CheckScalarWarnings(cfg)
	if out := collector.String(); out != "" {
		t.Errorf("expected no warnings when openapi disabled, got: %s", out)
	}
}

func TestCheckScalarWarnings_Configured_NoWarnings(t *testing.T) {
	collector := warningCollector(t)
	cfg := &ServiceConfig{
		Server: ServerConf{
			OpenAPI: &OpenAPIConf{Enabled: true},
			CORS: &CORSConf{
				Origins: []string{"https://app.example.com"},
			},
			SecurityHeaders: &SecurityHeadersConf{
				CSPConfig: &CSPConf{},
			},
		},
	}
	CheckScalarWarnings(cfg)
	if out := collector.String(); out != "" {
		t.Errorf("expected no warnings when CORS+CSP configured, got: %s", out)
	}
}

func TestCheckScalarWarnings_CORSGroup_NoCSP(t *testing.T) {
	collector := warningCollector(t)
	cfg := &ServiceConfig{
		Server: ServerConf{
			OpenAPI: &OpenAPIConf{Enabled: true},
			CORSGroups: []CORSGroupConf{
				{Name: "docs", Origins: []string{"*"}},
			},
		},
	}
	CheckScalarWarnings(cfg)
	out := collector.String()
	if strings.Contains(out, "no CORS configured") {
		t.Errorf("CORS group should satisfy the CORS requirement, got: %s", out)
	}
	if !strings.Contains(out, "no CSP configured") {
		t.Errorf("expected CSP warning still present, got: %s", out)
	}
}
