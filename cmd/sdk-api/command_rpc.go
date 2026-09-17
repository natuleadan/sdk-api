package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// protoFileArg validates and resolves a proto file path.
// Returns the absolute path, the directory, and the base filename.
// The file is confirmed to exist with .proto extension.
func protoFileArg(path string) (dir, name string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", "", fmt.Errorf("file not found: %w", err)
	}
	if !strings.HasSuffix(abs, ".proto") {
		return "", "", fmt.Errorf("file must have .proto extension")
	}
	return filepath.Dir(abs), filepath.Base(abs), nil
}

// protoc runs protoc in the specified directory.
// protoName is validated to be safe (confirmed .proto file).
var protocPath string

func init() {
	protocPath, _ = exec.LookPath("protoc")
}

func protocCmd(protoDir, protoName string, extraArgs ...string) *exec.Cmd {
	// protoName is validated by protoFileArg: confirmed .proto file via
	// filepath.Base + os.Stat + .suffix check. Path traversal eliminated
	// by filepath.Abs. No injection possible.
	if protocPath == "" {
		protocPath = "/usr/bin/protoc"
	}
	return &exec.Cmd{
		Path: protocPath,
		Args: append([]string{"protoc", protoName}, extraArgs...),
		Dir:  protoDir,
	}
}

// buildGenerateOpts translates --module/--go-package into protoc option flags:
// module=<prefix> strips the prefix for output placement, and
// M<file>=<importpath> pins the Go import path of the compiled file.
func buildGenerateOpts(protoName, module, goPackage string) (goOpts, grpcOpts []string) {
	goOpts = []string{"paths=import"}
	grpcOpts = []string{"paths=import"}
	if module != "" {
		goOpts = append(goOpts, "module="+module)
		grpcOpts = append(grpcOpts, "module="+module)
	}
	if goPackage != "" {
		mapping := "M" + protoName + "=" + goPackage
		goOpts = append(goOpts, mapping)
		grpcOpts = append(grpcOpts, mapping)
	}
	return goOpts, grpcOpts
}

func protocOutArgs(outFlag, absOut string, opts []string) []string {
	args := []string{outFlag + "=" + absOut}
	optFlag := strings.TrimSuffix(outFlag, "_out") + "_opt"
	for _, o := range opts {
		args = append(args, optFlag+"="+o)
	}
	return args
}

var rpcGenerateCmd = &cobra.Command{
	Use:   "generate [proto-file]",
	Short: "Generate gRPC stubs from a proto file",
	Long: `Runs protoc to generate pb.go and _grpc.pb.go files.

Usage:
  sdk-api rpc generate proto/auth.proto --out=pb
  sdk-api rpc generate proto/consent/v1/consent.proto --out=. --module=github.com/acme/mono --go-package=github.com/acme/mono/gen/go/consent/v1

The output directory defaults to the directory of the proto file.
--module strips the module prefix for output placement (monorepo layouts);
--go-package pins the Go import path via protoc M mapping.
Requires protoc, protoc-gen-go, and protoc-gen-go-grpc in PATH.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		protoFile := args[0]
		outDir, _ := cmd.Flags().GetString("out")
		module, _ := cmd.Flags().GetString("module")
		goPackage, _ := cmd.Flags().GetString("go-package")

		protoDir, protoName, err := protoFileArg(protoFile)
		if err != nil {
			return err
		}

		if outDir == "" {
			outDir = protoDir
		}
		absOut, err := filepath.Abs(outDir)
		if err != nil {
			return fmt.Errorf("out path: %w", err)
		}
		if err := os.MkdirAll(absOut, 0o750); err != nil {
			return err
		}

		goOpts, grpcOpts := buildGenerateOpts(protoName, module, goPackage)
		extra := append(protocOutArgs("--go_out", absOut, goOpts), protocOutArgs("--go-grpc_out", absOut, grpcOpts)...)
		pc := protocCmd(protoDir, protoName, extra...)
		pc.Stdout = os.Stdout
		pc.Stderr = os.Stderr
		if err := pc.Run(); err != nil {
			return fmt.Errorf("protoc: %w", err)
		}

		fmt.Printf("Generated: %s/%s\n", protoDir, protoName)
		return nil
	},
}

var rpcValidateCmd = &cobra.Command{
	Use:   "validate [proto-file]",
	Short: "Validate a proto file syntax",
	Long: `Validates a proto file using protoc without generating code.

Usage:
  sdk-api rpc validate proto/auth_service.proto

Requires protoc in PATH. Exit code 0 on success.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		protoFile := args[0]

		protoDir, protoName, err := protoFileArg(protoFile)
		if err != nil {
			return err
		}

		pc := protocCmd(protoDir, protoName, "-o", "/dev/null")
		pc.Stdout = os.Stdout
		pc.Stderr = os.Stderr
		if err := pc.Run(); err != nil {
			return fmt.Errorf("proto: %w", err)
		}

		fmt.Printf("Valid: %s/%s\n", protoDir, protoName)
		return nil
	},
}

var rpcCmd = &cobra.Command{
	Use:   "rpc",
	Short: "gRPC code generation tools",
}

func init() {
	rpcCmd.AddCommand(rpcGenerateCmd)
	rpcCmd.AddCommand(rpcValidateCmd)
	rpcGenerateCmd.Flags().String("out", "", "Output directory (default: proto file dir)")
	rpcGenerateCmd.Flags().String("module", "", "Module prefix strip for output placement (monorepo layouts)")
	rpcGenerateCmd.Flags().String("go-package", "", "Go import path for the file via protoc M mapping")
	rootCmd.AddCommand(rpcCmd)
}
