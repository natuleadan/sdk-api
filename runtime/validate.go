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
