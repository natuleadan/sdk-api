package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/natuleadan/sdk-api/infra/logx"
)

//go:embed templates/main.go.tmpl
var tmplMain string

//go:embed templates/service.yaml.tmpl
var tmplYAML string

//go:embed templates/model.go.tmpl
var tmplModel string

//go:embed templates/handler_list.go.tmpl
var tmplHandlerList string

//go:embed templates/handler_get.go.tmpl
var tmplHandlerGet string

//go:embed templates/handler_create.go.tmpl
var tmplHandlerCreate string

//go:embed templates/handler_update.go.tmpl
var tmplHandlerUpdate string

//go:embed templates/handler_delete.go.tmpl
var tmplHandlerDelete string

//go:embed templates/logic.go.tmpl
var tmplLogic string

//go:embed templates/svc.go.tmpl
var tmplSVC string

//go:embed templates/config.go.tmpl
var tmplConfig string

//go:embed templates/env.tmpl
var tmplEnv string

//go:embed templates/hooks.go.tmpl
var tmplHooks string

//go:embed templates/logic_test.go.tmpl
var tmplLogicTest string

//go:embed templates/middleware.go.tmpl
var tmplMiddleware string

//go:embed templates/proto.tmpl
var tmplProto string

//go:embed templates/grpc_server.go.tmpl
var tmplGrpcServer string

//go:embed templates/pb.go.tmpl
var tmplPB string

//go:embed templates/routes.go.tmpl
var tmplRoutes string

//go:embed templates/cache.go.tmpl
var tmplCache string

//go:embed templates/handler_crud_all.go.tmpl
var tmplCRUDAll string

//go:embed templates/handler_rest.go.tmpl
var tmplRestHandler string

//go:embed templates/handler_rest_all.go.tmpl
var tmplRestAll string

//go:embed templates/rest_routes.go.tmpl
var tmplRestRoutes string

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

type restEndpointDef struct {
	Method      string
	Path        string
	HandlerName string
	FuncName    string
	FileName    string
	Group       string
	SDKModule   string
	ModulePath  string
}

type fieldDef struct {
	Name     string
	Type     string
	Column   string
	JSON     string
	ProtoIdx int
}

type newConfig struct {
	ServiceName   string
	ModelName     string
	ResourceName  string
	ResourcePath  string
	DBName        string
	Port          int
	GrpcPort      int
	GrpcEnabled   bool
	WithTests     bool
	Dir           string
	Consumers     []consumerDef
	Producers     []producerDef
	ExitWorkers   []exitWorkerDef
	CronJobs      []cronJobDef
	RestEndpoints []restEndpointDef
	ExtraFields   []fieldDef
	SDKModule     string
	ModulePath    string
	DBTable       string
	HasDB         bool
	HasRest       bool
	HasNATS       bool
	HasCache      bool
	CacheKV       string
	Split         bool
	StreamNames   []string
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

	parseNewFlags(args[1:], &cfg)

	if cfg.ModelName == "" {
		cfg.ModelName = pascalCase(cfg.ServiceName)
	}
	if cfg.Dir == "" {
		cfg.Dir = cfg.ServiceName
	}
	if err := finalizeConfig(&cfg); err != nil {
		return err
	}
	return generate(cfg)
}

func parseNewFlags(flags []string, cfg *newConfig) {
	flagHandlers := map[string]func([]string, int, *newConfig) int{
		"--model":   handleModelFlag,
		"--port":    handlePortFlag,
		"--fields":  handleFieldsFlag,
		"--consume": handleConsumeFlag,
		"--publish": handlePublishFlag,
		"--exit":    handleExitFlag,
		"--cron":    handleCronFlag,
		"--cache":   handleCacheFlag,
		"--rest":    handleRestFlag,
		"--dir":     handleDirFlag,
	}
	boolFlags := map[string]*bool{
		"--grpc":       &cfg.GrpcEnabled,
		"--with-tests": &cfg.WithTests,
		"--split":      &cfg.Split,
	}

	for i := 0; i < len(flags); i++ {
		if flags[i] == "--grpc-port" && i+1 < len(flags) {
			i++
			if _, err := fmt.Sscanf(flags[i], "%d", &cfg.GrpcPort); err != nil {
				logx.Errorf("parse grpc-port: %v", err)
			}
			continue
		}
		if fn, ok := flagHandlers[flags[i]]; ok {
			i = fn(flags, i, cfg)
			continue
		}
		if ptr, ok := boolFlags[flags[i]]; ok {
			*ptr = true
			continue
		}
	}
}

func handleCacheFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		cfg.CacheKV = args[i]
		cfg.HasCache = true
	}
	return i
}

func finalizeConfig(cfg *newConfig) error {
	cfg.HasNATS = len(cfg.Consumers) > 0 || len(cfg.Producers) > 0 || len(cfg.ExitWorkers) > 0 || len(cfg.CronJobs) > 0
	cfg.HasDB = cfg.ModelName != ""
	cfg.HasRest = len(cfg.RestEndpoints) > 0
	for i := range cfg.RestEndpoints {
		cfg.RestEndpoints[i].SDKModule = cfg.SDKModule
		cfg.RestEndpoints[i].ModulePath = cfg.ModulePath
	}
	cfg.StreamNames = unique(cfg.StreamNames)
	cfg.DBTable = toSnake(cfg.ModelName)
	cfg.ResourceName = plural(cfg.DBTable)
	cfg.ResourcePath = "/" + cfg.ResourceName
	if cfg.HasRest && cfg.HasDB {
		for _, ep := range cfg.RestEndpoints {
			if ep.Group == cfg.ResourceName {
				return fmt.Errorf("rest group %q conflicts with crud resource %q", ep.Group, cfg.ResourceName)
			}
		}
	}
	if cfg.HasRest {
		seen := map[string]bool{}
		for _, ep := range cfg.RestEndpoints {
			if seen[ep.Group] {
				return fmt.Errorf("duplicate rest group %q", ep.Group)
			}
			seen[ep.Group] = true
		}
	}
	if cfg.DBName == "" {
		cfg.DBName = "pg-main"
	}
	if cfg.GrpcPort == 0 {
		cfg.GrpcPort = cfg.Port + 1
	}
	return nil
}

func handleModelFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		cfg.ModelName = args[i]
	}
	return i
}

func handlePortFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		if _, err := fmt.Sscanf(args[i], "%d", &cfg.Port); err != nil {
			logx.Errorf("new: parse port error: %v", err)
		}
	}
	return i
}

func handleFieldsFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		idx := 2 // start after id
		for f := range strings.SplitSeq(args[i], ",") {
			f = strings.TrimSpace(f)
			parts := strings.SplitN(f, ":", 2)
			if len(parts) != 2 {
				continue
			}
			name := strings.TrimSpace(parts[0])
			typ := strings.TrimSpace(parts[1])
			cfg.ExtraFields = append(cfg.ExtraFields, fieldDef{
				Name:     pascalCase(name),
				Type:     goType(typ),
				Column:   toSnake(name),
				JSON:     toSnake(name),
				ProtoIdx: idx,
			})
			idx++
		}
	}
	return i
}

func handleConsumeFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		for c := range strings.SplitSeq(args[i], ",") {
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
				cfg.ExitWorkers = append(cfg.ExitWorkers, exitWorkerDef{
					Name:    cd.Durable,
					Stream:  cd.Stream,
					Handler: cd.Handler,
				})
			}
		}
	}
	return i
}

func handlePublishFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		for p := range strings.SplitSeq(args[i], ",") {
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
	return i
}

func handleExitFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		for e := range strings.SplitSeq(args[i], ",") {
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
	return i
}

func handleCronFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		for c := range strings.SplitSeq(args[i], ",") {
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
	return i
}

func handleRestFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		for r := range strings.SplitSeq(args[i], ",") {
			r = strings.TrimSpace(r)
			groupRest := strings.SplitN(r, "|", 2)
			group := ""
			rest := groupRest[0]
			if len(groupRest) == 2 {
				group = groupRest[0]
				rest = groupRest[1]
			}
			parts := strings.SplitN(rest, ":", 3)
			if len(parts) != 3 {
				continue
			}
			method := strings.TrimSpace(parts[0])
			path := strings.TrimSpace(parts[1])
			handler := strings.TrimSpace(parts[2])
			if group == "" {
				group = "rest"
			}
			cfg.RestEndpoints = append(cfg.RestEndpoints, restEndpointDef{
				Method:      method,
				Path:        path,
				HandlerName: handler,
				FuncName:    pascalCase(handler),
				FileName:    toSnake(handler) + ".go",
				Group:       group,
			})
		}
	}
	return i
}

func handleDirFlag(args []string, i int, cfg *newConfig) int {
	if i+1 < len(args) {
		i++
		cfg.Dir = args[i]
	}
	return i
}

type tmplDef struct {
	rel  string
	src  string
	data any
}

func collectTemplates(cfg newConfig) ([]tmplDef, []string) {
	files := []tmplDef{
		{rel: "cmd/main.go", src: tmplMain},
		{rel: "service.yaml", src: tmplYAML},
		{rel: "models/model.go", src: tmplModel},
		{rel: "internal/config/config.go", src: tmplConfig},
		{rel: "internal/svc/servicecontext.go", src: tmplSVC},
		{rel: "internal/middleware/custom.go", src: tmplMiddleware},
		{rel: ".env", src: tmplEnv},
	}

	if cfg.HasDB {
		if cfg.Split {
			r := cfg.ResourceName
			files = append(files,
				tmplDef{rel: "internal/handler/list_" + r + ".go", src: tmplHandlerList},
				tmplDef{rel: "internal/handler/get_" + r + ".go", src: tmplHandlerGet},
				tmplDef{rel: "internal/handler/create_" + r + ".go", src: tmplHandlerCreate},
				tmplDef{rel: "internal/handler/update_" + r + ".go", src: tmplHandlerUpdate},
				tmplDef{rel: "internal/handler/delete_" + r + ".go", src: tmplHandlerDelete},
			)
		} else {
			files = append(files, tmplDef{
				rel: "internal/handler/" + cfg.ResourceName + ".go", src: tmplCRUDAll,
			})
		}
		files = append(files,
			tmplDef{rel: "internal/logic/" + cfg.ResourceName + ".go", src: tmplLogic},
			tmplDef{rel: "models/" + cfg.ResourceName + "_hooks.go", src: tmplHooks},
		)
	}

	if cfg.WithTests && cfg.HasDB {
		files = append(files, tmplDef{
			rel: "internal/logic/" + cfg.ResourceName + "_test.go", src: tmplLogicTest,
		})
	}

	var extraDirs []string
	if cfg.HasCache {
		extraDirs = append(extraDirs, filepath.Join(cfg.Dir, "internal", "cache"))
		files = append(files, tmplDef{rel: "internal/cache/invalidate.go", src: tmplCache})
	}

	files = append(files, tmplDef{rel: "internal/handler/routes.go", src: tmplRoutes})

	if cfg.HasRest {
		files = append(files, tmplDef{rel: "internal/handler/rest_routes.go", src: tmplRestRoutes})
		if cfg.Split {
			for _, ep := range cfg.RestEndpoints {
				fn := ep.Group + "_" + toSnake(ep.HandlerName)
				ep.FuncName = pascalCase(ep.HandlerName)
				files = append(files, tmplDef{rel: "internal/handler/" + fn + ".go", src: tmplRestHandler, data: ep})
			}
		} else {
			groups := map[string][]restEndpointDef{}
			for _, ep := range cfg.RestEndpoints {
				g := ep.Group
				groups[g] = append(groups[g], ep)
			}
			for group, endpoints := range groups {
				sub := cfg
				sub.RestEndpoints = endpoints
				files = append(files, tmplDef{rel: "internal/handler/" + group + ".go", src: tmplRestAll, data: &sub})
			}
		}
	}

	if cfg.GrpcEnabled {
		for _, d := range []string{"grpcserver", "pb", "api"} {
			extraDirs = append(extraDirs, filepath.Join(cfg.Dir, d))
		}
		files = append(files,
			tmplDef{rel: "api/" + cfg.ResourceName + ".proto", src: tmplProto},
			tmplDef{rel: "pb/" + cfg.ResourceName + ".pb.go", src: tmplPB},
			tmplDef{rel: "grpcserver/" + cfg.ResourceName + ".go", src: tmplGrpcServer},
		)
	}

	return files, extraDirs
}

func protoType(goType string) string {
	switch goType {
	case "int", "int64":
		return "int64"
	case "float64", "float32":
		return "double"
	case "string":
		return "string"
	case "bool":
		return "bool"
	case "time.Time":
		return "string"
	default:
		return "string"
	}
}

func generate(cfg newConfig) error {
	dirs := []string{
		cfg.Dir,
		filepath.Join(cfg.Dir, "cmd"),
		filepath.Join(cfg.Dir, "internal", "config"),
		filepath.Join(cfg.Dir, "internal", "handler"),
		filepath.Join(cfg.Dir, "internal", "logic"),
		filepath.Join(cfg.Dir, "internal", "svc"),
		filepath.Join(cfg.Dir, "internal", "middleware"),
		filepath.Join(cfg.Dir, "models"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	files, extraDirs := collectTemplates(cfg)
	for _, d := range extraDirs {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	funcMap := template.FuncMap{
		"protoType": protoType,
		"add":       func(a, b int) int { return a + b },
	}

	for _, f := range files {
		t, err := template.New(f.rel).Funcs(funcMap).Parse(f.src)
		if err != nil {
			return fmt.Errorf("template %s: %w", f.rel, err)
		}
		var buf bytes.Buffer
		var tmplData any = cfg
		if f.data != nil {
			tmplData = f.data
		}
		if err := t.Execute(&buf, tmplData); err != nil {
			return fmt.Errorf("execute %s: %w", f.rel, err)
		}
		path := filepath.Join(cfg.Dir, f.rel)
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", f.rel, err)
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
		switch {
		case ch >= 'A' && ch <= 'Z':
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
		case ch == '-':
			b.WriteRune('_')
		default:
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
