package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// findSharedProto walks up from dir looking for a monorepo shared proto/
// folder (marked by proto/buf.yaml, or a proto/ folder holding .proto files).
// It returns the monorepo root (the parent of proto/) or "".
func findSharedProto(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for range 12 {
		protoDir := filepath.Join(abs, "proto")
		if st, err := os.Stat(filepath.Join(protoDir, "buf.yaml")); err == nil && !st.IsDir() {
			return abs
		}
		if st, err := os.Stat(protoDir); err == nil && st.IsDir() && hasProtoFiles(protoDir) {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
	return ""
}

// hasProtoFiles reports whether any .proto file exists under dir.
func hasProtoFiles(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(path, ".proto") {
			found = true
		}
		return nil
	})
	return found
}

// discoverSharedProtos lists .proto files under root/proto, relative to it
// (e.g. consent/v1/consent.proto), sorted for deterministic output.
func discoverSharedProtos(root string) ([]string, error) {
	protoDir := filepath.Join(root, "proto")
	var out []string
	err := filepath.WalkDir(protoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".proto") {
			return nil
		}
		rel, err := filepath.Rel(protoDir, path)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// moduleOf reads the `module X` line from a go.mod file.
func moduleOf(goModPath string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(goModPath))
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if mod, ok := strings.CutPrefix(line, "module "); ok {
			mod = strings.TrimSpace(mod)
			if mod != "" {
				return mod, nil
			}
		}
	}
	return "", fmt.Errorf("no module line in %s", goModPath)
}

// genGoModule reads the canonical module from gen/go/go.mod, present when
// the monorepo was initialized with 'sdk-api proto init'.
func genGoModule(root string) (string, error) {
	return moduleOf(filepath.Join(root, "gen", "go", "go.mod"))
}

// resolveGenModule returns the canonical gen/go module: explicit flag first,
// then the existing gen/go/go.mod, then root go.mod + /gen/go.
func resolveGenModule(root, flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if mod, err := moduleOf(filepath.Join(root, "gen", "go", "go.mod")); err == nil {
		return mod, nil
	}
	if mod, err := moduleOf(filepath.Join(root, "go.mod")); err == nil {
		return mod + "/gen/go", nil
	}
	return "", fmt.Errorf("cannot derive gen module: pass --module or add a root go.mod")
}

// goMinor returns the toolchain minor version (e.g. "1.27") for scaffolding.
func goMinor() string {
	v := strings.TrimPrefix(runtime.Version(), "go")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return "1.27"
	}
	return parts[0] + "." + parts[1]
}

func writeProtoFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

const bufYAMLTemplate = `version: v2
lint:
  use:
    - DEFAULT
breaking:
  use:
    - FILE
`

const bufGenTemplate = `# Canonical output: every shared proto compiles into the gen/go module.
# Vendored per-service copies are produced by: sdk-api proto generate
version: v2
plugins:
  - plugin: go
    out: gen/go
    opt: paths=import
  - plugin: go-grpc
    out: gen/go
    opt: paths=import
`

const protoREADMETemplate = `# Shared protobuf contracts

` + "`proto/` holds the single source of truth for gRPC contracts " + `(` + "`<service>/v1/<service>.proto`" + `).
Each service vendors its compiled stubs into its own ` + "`pb/`" + ` directory
(Docker-context safe: no ` + "`replace ../proto`" + `).

Workflow:

    # scaffold the shared folder (once per monorepo)
    sdk-api proto init --module <gen-module>

    # scaffold a service against the shared contracts
    sdk-api new <svc> --grpc --dir <svc>

    # regenerate canonical gen/go + all vendored pb/ copies
    sdk-api proto generate
`

func runProtoInit(root, moduleFlag string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}
	if st, err := os.Stat(filepath.Join(abs, "proto", "buf.yaml")); err == nil && !st.IsDir() {
		return fmt.Errorf("proto/buf.yaml already exists: monorepo already initialized")
	}
	genMod, err := resolveGenModule(abs, moduleFlag)
	if err != nil {
		return err
	}
	if err := writeProtoFile(filepath.Join(abs, "proto", "buf.yaml"), bufYAMLTemplate); err != nil {
		return err
	}
	if err := writeProtoFile(filepath.Join(abs, "proto", "buf.gen.yaml"), bufGenTemplate); err != nil {
		return err
	}
	goMod := "module " + genMod + "\n\ngo " + goMinor() + "\n"
	if err := writeProtoFile(filepath.Join(abs, "gen", "go", "go.mod"), goMod); err != nil {
		return err
	}
	if err := writeProtoFile(filepath.Join(abs, "proto", "README.md"), protoREADMETemplate); err != nil {
		return err
	}
	fmt.Printf("Initialized shared proto contracts in %s (gen module %s)\n", abs, genMod)
	return nil
}

// vendTarget maps one shared proto file to a vendored service pb/ directory.
type vendTarget struct {
	relProto string // e.g. consent/v1/consent.proto (relative to proto/)
	svcDir   string // absolute service directory holding pb/
	svcMod   string // service go module (for M mappings)
}

// resolveVendTargets pairs shared protos with vendored outputs: explicit
// --vend rel=dir pairs first, then the same-name convention <root>/<top>/pb.
func resolveVendTargets(root string, rels, vends []string) ([]vendTarget, error) {
	explicit := map[string]string{}
	for _, v := range vends {
		rel, dir, ok := strings.Cut(v, "=")
		if !ok || rel == "" || dir == "" {
			return nil, fmt.Errorf("bad --vend %q (want rel-proto=svc-dir)", v)
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve vend dir: %w", err)
		}
		explicit[filepath.ToSlash(rel)] = abs
	}
	var out []vendTarget
	for _, rel := range rels {
		svcDir, ok := explicit[rel]
		if !ok {
			top, _, _ := strings.Cut(rel, "/")
			candidate := filepath.Join(root, top, "pb")
			if st, err := os.Stat(candidate); err != nil || !st.IsDir() {
				continue
			}
			svcDir = filepath.Join(root, top)
		}
		svcMod, err := moduleOf(filepath.Join(svcDir, "go.mod"))
		if err != nil {
			fmt.Printf("warn: skip vendored copy for %s: %v\n", rel, err)
			continue
		}
		out = append(out, vendTarget{relProto: rel, svcDir: svcDir, svcMod: svcMod})
	}
	return out, nil
}

// canonicalProtocArgs builds the protoc invocation compiling one shared proto
// into the canonical gen/go tree (module strip, out at the monorepo root).
func canonicalProtocArgs(protoRoot, rel, genOut, genModule string) []string {
	return []string{
		"protoc", rel,
		"--proto_path=" + protoRoot,
		"--go_out=" + genOut,
		"--go_opt=module=" + genModule,
		"--go-grpc_out=" + genOut,
		"--go-grpc_opt=module=" + genModule,
	}
}

// vendoredProtocArgs builds the protoc invocation compiling one shared proto
// into a service vendored pb/ copy (M mapping + module strip, out at svc root).
func vendoredProtocArgs(protoRoot, rel, svcDir, svcModule string) []string {
	mapping := "M" + rel + "=" + svcModule + "/pb"
	return []string{
		"protoc", rel,
		"--proto_path=" + protoRoot,
		"--go_out=" + svcDir,
		"--go_opt=module=" + svcModule,
		"--go_opt=" + mapping,
		"--go-grpc_out=" + svcDir,
		"--go-grpc_opt=module=" + svcModule,
		"--go-grpc_opt=" + mapping,
	}
}

func runProtoCmd(args []string, dir string) error {
	if protocPath == "" {
		return fmt.Errorf("protoc not found in PATH")
	}
	pc := &exec.Cmd{Path: protocPath, Args: args, Dir: dir}
	pc.Stdout = os.Stdout
	pc.Stderr = os.Stderr
	if err := pc.Run(); err != nil {
		return fmt.Errorf("protoc %s: %w", strings.Join(args[1:], " "), err)
	}
	return nil
}

func runBufGenerate(root string) error {
	bufPath, err := exec.LookPath("buf")
	if err != nil {
		return fmt.Errorf("buf not found in PATH")
	}
	pc := &exec.Cmd{
		Path: bufPath,
		Args: []string{"buf", "generate", "proto", "--template", "proto/buf.gen.yaml"},
		Dir:  root,
	}
	pc.Stdout = os.Stdout
	pc.Stderr = os.Stderr
	if err := pc.Run(); err != nil {
		return fmt.Errorf("buf generate: %w", err)
	}
	return nil
}

func runProtoGenerate(root, moduleFlag string, vends []string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}
	rels, err := discoverSharedProtos(abs)
	if err != nil {
		return err
	}
	if len(rels) == 0 {
		return fmt.Errorf("no .proto files under %s", filepath.Join(abs, "proto"))
	}
	genMod, err := resolveGenModule(abs, moduleFlag)
	if err != nil {
		return err
	}
	protoRoot := filepath.Join(abs, "proto")
	genOut := filepath.Join(abs, "gen", "go")

	hasBuf := lookPathOK("buf")
	if !hasBuf && protocPath == "" {
		return fmt.Errorf("need buf or protoc in PATH to regenerate (canonical %s)", genMod)
	}
	if err := runCanonicalPass(abs, protoRoot, rels, genOut, genMod, hasBuf); err != nil {
		return err
	}
	return runVendoredPass(abs, protoRoot, rels, vends)
}

// runCanonicalPass regenerates the shared gen/go output via buf or protoc.
func runCanonicalPass(abs, protoRoot string, rels []string, genOut, genMod string, hasBuf bool) error {
	if hasBuf {
		return runBufGenerate(abs)
	}
	for _, rel := range rels {
		if err := runProtoCmd(canonicalProtocArgs(protoRoot, rel, genOut, genMod), protoRoot); err != nil {
			return err
		}
	}
	return nil
}

// runVendoredPass regenerates one pb/ copy per mapped service (needs protoc).
func runVendoredPass(abs, protoRoot string, rels, vends []string) error {
	targets, err := resolveVendTargets(abs, rels, vends)
	if err != nil {
		return err
	}
	if protocPath == "" {
		if len(targets) > 0 {
			fmt.Printf("warn: protoc not found, skipped %d vendored copies (need protoc in PATH)\n", len(targets))
		}
		return nil
	}
	for _, t := range targets {
		if err := runProtoCmd(vendoredProtocArgs(protoRoot, t.relProto, t.svcDir, t.svcMod), protoRoot); err != nil {
			return err
		}
		fmt.Printf("Vendored: %s -> %s/pb\n", t.relProto, t.svcDir)
	}
	return nil
}

// lookPathOK reports whether a tool is available in PATH.
func lookPathOK(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

var protoInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a shared proto/ contracts folder for a monorepo",
	Long: `Creates proto/buf.yaml, proto/buf.gen.yaml, gen/go/go.mod and proto/README.md.

Usage:
  sdk-api proto init --dir /path/to/monorepo --module example.com/mono/gen/go

The gen module defaults to <root go.mod module>/gen/go (or the existing
gen/go/go.mod). Services created later with 'sdk-api new --grpc' inside the
monorepo write their contract to proto/<service>/v1/ and vendor the compiled
pb/ copy (Docker-context safe, no replace ../proto).`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		dir, _ := cmd.Flags().GetString("dir")
		module, _ := cmd.Flags().GetString("module")
		if dir == "" {
			dir = "."
		}
		return runProtoInit(dir, module)
	},
}

var protoGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Regenerate canonical gen/go and all vendored pb/ copies",
	Long: `Regenerates every shared proto/ contract: canonical output into gen/go
(via buf when available, else protoc with module strip) plus one vendored
pb/ copy per service (protoc with M mapping + module strip).

Vendored targets resolve by the same-name convention <root>/<top>/pb, or
explicitly for legacy mismatches:

  sdk-api proto generate --vend consent/v1/consent.proto=ms-consent

Requires buf or protoc in PATH (protoc for the vendored copies).`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		dir, _ := cmd.Flags().GetString("dir")
		module, _ := cmd.Flags().GetString("module")
		vends, _ := cmd.Flags().GetStringArray("vend")
		if dir == "" {
			dir = "."
		}
		return runProtoGenerate(dir, module, vends)
	},
}

var protoCmd = &cobra.Command{
	Use:   "proto",
	Short: "Shared protobuf contracts for monorepos",
}

func init() {
	protoInitCmd.Flags().String("dir", "", "Monorepo root (default: current directory)")
	protoInitCmd.Flags().String("module", "", "Canonical gen/go module (default: derived)")
	protoGenerateCmd.Flags().String("dir", "", "Monorepo root (default: current directory)")
	protoGenerateCmd.Flags().String("module", "", "Canonical gen/go module (default: derived)")
	protoGenerateCmd.Flags().StringArray("vend", []string{}, "Explicit vendored target rel-proto=svc-dir (repeatable)")
	protoCmd.AddCommand(protoInitCmd)
	protoCmd.AddCommand(protoGenerateCmd)
	rootCmd.AddCommand(protoCmd)
}
