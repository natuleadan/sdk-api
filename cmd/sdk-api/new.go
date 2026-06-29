package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/main.go.tmpl
var tmplMain string

//go:embed templates/service.yaml.tmpl
var tmplYAML string

//go:embed templates/model.go.tmpl
var tmplModel string

type consumerDef struct {
	Stream  string
	Durable string
	Handler string
}

type producerDef struct {
	Stream string
	After  []string
}

type exitWorkerDef struct {
	Name    string
	Stream  string
	Handler string
}

type cronJobDef struct {
	Name    string
	Handler string
}

type fieldDef struct {
	Name   string
	Type   string
	Column string
	JSON   string
}

type newConfig struct {
	ServiceName  string
	ModelName    string
	ResourceName string
	Port         int
	Dir          string
	Consumers    []consumerDef
	Producers    []producerDef
	ExitWorkers  []exitWorkerDef
	CronJobs     []cronJobDef
	ExtraFields  []fieldDef
	SDKModule    string
	ModulePath   string
	DBTable      string
	HasDB        bool
	HasNATS      bool
	StreamNames  []string
}

func runNew(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("service name is required")
	}

	cfg := newConfig{
		ServiceName: args[0],
		Port:        8080,
		SDKModule:   "github.com/natuleadan/sdk-api",
		ModulePath:  "github.com/natuleadan/" + args[0],
	}

	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--model":
			if i+1 < len(rest) {
				i++
				cfg.ModelName = rest[i]
			}
		case "--port":
			if i+1 < len(rest) {
				i++
				fmt.Sscanf(rest[i], "%d", &cfg.Port)
			}
		case "--fields":
			if i+1 < len(rest) {
				i++
				for _, f := range strings.Split(rest[i], ",") {
					f = strings.TrimSpace(f)
					parts := strings.SplitN(f, ":", 2)
					if len(parts) != 2 {
						continue
					}
					name := strings.TrimSpace(parts[0])
					typ := strings.TrimSpace(parts[1])
					cfg.ExtraFields = append(cfg.ExtraFields, fieldDef{
						Name:   pascalCase(name),
						Type:   goType(typ),
						Column: toSnake(name),
						JSON:   toSnake(name),
					})
				}
			}
		case "--consume":
			if i+1 < len(rest) {
				i++
				for _, c := range strings.Split(rest[i], ",") {
					parts := strings.SplitN(c, ":", 3)
					if len(parts) >= 2 {
						cd := consumerDef{
							Stream:  strings.TrimSpace(parts[0]),
							Durable: strings.TrimSpace(parts[1]),
						}
						if len(parts) >= 3 {
							cd.Handler = strings.TrimSpace(parts[2])
						} else {
							cd.Handler = "on" + pascalCase(cd.Stream)
						}
						cfg.Consumers = append(cfg.Consumers, cd)
						cfg.StreamNames = append(cfg.StreamNames, cd.Stream)
						// Map to exit workers for v2 YAML
						cfg.ExitWorkers = append(cfg.ExitWorkers, exitWorkerDef{
							Name:    cd.Durable,
							Stream:  cd.Stream,
							Handler: cd.Handler,
						})
					}
				}
			}
		case "--publish":
			if i+1 < len(rest) {
				i++
				for _, p := range strings.Split(rest[i], ",") {
					parts := strings.SplitN(p, ":", 2)
					pd := producerDef{Stream: strings.TrimSpace(parts[0])}
					if len(parts) >= 2 {
						pd.After = strings.Split(strings.TrimSpace(parts[1]), "|")
					} else {
						pd.After = []string{"create", "update"}
					}
					cfg.Producers = append(cfg.Producers, pd)
					cfg.StreamNames = append(cfg.StreamNames, pd.Stream)
				}
			}
		case "--exit":
			if i+1 < len(rest) {
				i++
				for _, e := range strings.Split(rest[i], ",") {
					parts := strings.SplitN(e, ":", 3)
					if len(parts) >= 2 {
						ed := exitWorkerDef{
							Stream:  strings.TrimSpace(parts[0]),
							Handler: strings.TrimSpace(parts[1]),
						}
						if len(parts) >= 3 {
							ed.Name = strings.TrimSpace(parts[2])
						} else {
							ed.Name = ed.Stream + "-worker"
						}
						cfg.ExitWorkers = append(cfg.ExitWorkers, ed)
						cfg.StreamNames = append(cfg.StreamNames, ed.Stream)
					}
				}
			}
		case "--cron":
			if i+1 < len(rest) {
				i++
				for _, c := range strings.Split(rest[i], ",") {
					parts := strings.SplitN(c, ":", 2)
					cj := cronJobDef{Handler: strings.TrimSpace(parts[0])}
					if len(parts) >= 2 {
						cj.Name = strings.TrimSpace(parts[1])
					} else {
						cj.Name = cj.Handler
					}
					cfg.CronJobs = append(cfg.CronJobs, cj)
				}
			}
		case "--dir":
			if i+1 < len(rest) {
				i++
				cfg.Dir = rest[i]
			}
		}
	}

	if cfg.ModelName == "" {
		cfg.ModelName = pascalCase(cfg.ServiceName)
	}
	if cfg.Dir == "" {
		cfg.Dir = cfg.ServiceName
	}

	cfg.HasNATS = len(cfg.Consumers) > 0 || len(cfg.Producers) > 0 || len(cfg.ExitWorkers) > 0 || len(cfg.CronJobs) > 0
	cfg.HasDB = cfg.ModelName != ""
	cfg.StreamNames = unique(cfg.StreamNames)
	cfg.DBTable = toSnake(cfg.ModelName)
	cfg.ResourceName = plural(cfg.DBTable)

	return generate(cfg)
}

func generate(cfg newConfig) error {
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	modelDir := filepath.Join(cfg.Dir, "models")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}

	files := map[string]string{
		"main.go":         tmplMain,
		"service.yaml":    tmplYAML,
		"models/model.go": tmplModel,
	}

	for relPath, tmpl := range files {
		t, err := template.New(relPath).Parse(tmpl)
		if err != nil {
			return fmt.Errorf("template %s: %w", relPath, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, cfg); err != nil {
			return fmt.Errorf("execute %s: %w", relPath, err)
		}
		path := filepath.Join(cfg.Dir, relPath)
		if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
		fmt.Printf("  created %s\n", path)
	}

	return nil
}

func pascalCase(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func toSnake(s string) string {
	var b strings.Builder
	r := []rune(s)
	for i, ch := range r {
		if ch >= 'A' && ch <= 'Z' {
			if i > 0 {
				prev := r[i-1]
				hasNext := i+1 < len(r)
				prevLower := prev >= 'a' && prev <= 'z'
				nextLower := hasNext && r[i+1] >= 'a' && r[i+1] <= 'z'
				if prevLower || nextLower {
					b.WriteRune('_')
				}
			}
			b.WriteRune(ch + 32)
		} else if ch == '-' {
			b.WriteRune('_')
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func goType(t string) string {
	switch t {
	case "string":
		return "string"
	case "int", "int32":
		return "int"
	case "int64":
		return "int64"
	case "float", "float32", "float64":
		return "float64"
	case "bool":
		return "bool"
	case "time":
		return "time.Time"
	default:
		return "string"
	}
}

func plural(s string) string {
	if s == "" || s[len(s)-1] == 's' {
		return s
	}
	if s[len(s)-1] == 'y' {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

func unique(s []string) []string {
	seen := make(map[string]bool)
	var r []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			r = append(r, v)
		}
	}
	return r
}
