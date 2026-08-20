package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/natuleadan/sdk-api/infra/logx"
)

const (
	TargetAuto   = "auto"
	TargetVercel = "vercel"
	TargetDocker = "docker"
	TargetKube   = "kube"
	TargetBare   = "bare-metal"
)

func validateServerMode(cfg *ServiceConfig) error {
	mode := cfg.Server.Mode
	if mode == "" {
		cfg.Server.Mode = "monolith"
		mode = "monolith"
	}
	if mode != "monolith" && mode != "micro" {
		return fmt.Errorf("server.mode must be 'monolith' or 'micro' (got %q)", mode)
	}
	if mode == "monolith" {
		if cfg.Server.GrpcServer != nil && cfg.Server.GrpcServer.ListenOn != "" {
			logx.Infof("server.mode is monolith: grpc_server configured but will not start")
		}
	}
	return nil
}

var validTargets = map[string]bool{
	TargetAuto:   true,
	TargetVercel: true,
	TargetDocker: true,
	TargetKube:   true,
	TargetBare:   true,
}

func validateConfigDeploy(cfg *ServiceConfig) error {
	if cfg.Deploy == nil {
		return nil
	}
	target := cfg.Deploy.Target
	if target == "" || target == TargetAuto {
		return nil
	}
	if !validTargets[target] {
		return fmt.Errorf("deploy.target: invalid value %q (valid: auto, vercel, docker, kube, bare-metal)", target)
	}
	if cfg.Server.Prefork && target == TargetVercel {
		return fmt.Errorf("deploy.target=vercel: server.prefork must be false (Vercel does not support SO_REUSEPORT)")
	}
	if cfg.Server.TLS != nil && cfg.Server.TLS.Enabled && target == TargetVercel {
		return fmt.Errorf("deploy.target=vercel: server.tls.enabled must be false (Vercel terminates TLS at edge)")
	}
	return nil
}

// ValidateProjectStructure checks that the project at yamlPath is structurally
// compatible with a given deploy target. Used by CLI commands.
// yamlPath is the path to service.yaml; the project root is derived from it.
func ValidateProjectStructure(yamlPath string, target string) error {
	if target != TargetVercel {
		return nil
	}
	projectDir := filepath.Dir(yamlPath)
	if projectDir == "." {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			projectDir = "."
		}
	}
	goMod := filepath.Join(projectDir, "go.mod")
	if _, err := os.Stat(goMod); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("vercel requires go.mod at project root (not found in %s)", projectDir)
		}
		return fmt.Errorf("check go.mod: %w", err)
	}
	candidates := []string{
		"main.go",
		"cmd/api/main.go",
		"cmd/server/main.go",
	}
	found := false
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(projectDir, c)); err == nil {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("vercel requires an entrypoint: main.go, cmd/api/main.go, or cmd/server/main.go (none found in %s)", projectDir)
	}
	return nil
}

// CheckVercelWarnings logs non-blocking warnings for Vercel deployment.
func CheckVercelWarnings(cfg *ServiceConfig) {
	if cfg.Deploy == nil || cfg.Deploy.Target != TargetVercel {
		return
	}
	for _, entry := range cfg.Entry {
		if entry.Storage != nil && strings.EqualFold(entry.Storage.Mode, "local") {
			logx.Infof("deploy.target=vercel: entry %q uses local storage — files are ephemeral on Vercel", entry.Path)
		}
	}
}

// CheckScalarWarnings logs non-blocking warnings when Scalar UI is enabled but
// CORS or CSP are not configured for /docs. Scalar loads its assets from
// cdn.jsdelivr.net and Google Fonts; without the right CORS/CSP the docs page
// will render broken. The SDK never injects these for you — it is a user
// decision — so it warns and points to the reference example instead.
func CheckScalarWarnings(cfg *ServiceConfig) {
	oai := cfg.Server.OpenAPI
	if oai == nil || !oai.Enabled {
		return
	}
	sc := cfg.Server
	hasCORS := sc.CORS != nil && len(sc.CORS.Origins) > 0
	hasGroup := len(sc.CORSGroups) > 0
	docsPath := oai.DocsPath
	if docsPath == "" {
		docsPath = "/docs"
	}
	if !hasCORS && !hasGroup {
		logx.Infof("openapi: Scalar UI enabled at %q but no CORS configured — the docs page won't load from another origin. See examples/102-scalar-ui for the CORS/CSP model.", docsPath)
	}
	if sc.SecurityHeaders == nil || sc.SecurityHeaders.CSPConfig == nil {
		logx.Infof("openapi: Scalar UI enabled but no CSP configured — scripts from cdn.jsdelivr.net and Google Fonts will be blocked. See examples/102-scalar-ui for the required csp_config.")
	}
}
