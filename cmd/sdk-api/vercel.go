package main

import (
	_ "embed"
	"fmt"
	"os"
	"text/template"

	"github.com/natuleadan/sdk-api/infra/logx"

	"github.com/natuleadan/sdk-api/runtime"
)

//go:embed templates/vercel.json.tmpl
var tmplVercel string

type vercelConfig struct {
	BuildCmd string
	GoFlags  string
}

type vercelFlags struct {
	ConfigPath string
	OutputPath string
	BuildCmd   string
	GoFlags    string
}

func parseVercelFlags(args []string) vercelFlags {
	f := vercelFlags{
		ConfigPath: "service.yaml",
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			i = parseStrArg(args, i, &f.ConfigPath)
		case "--output":
			i = parseStrArg(args, i, &f.OutputPath)
		case "--build-command":
			i = parseStrArg(args, i, &f.BuildCmd)
		case "--go-flags":
			i = parseStrArg(args, i, &f.GoFlags)
		}
	}
	return f
}

func parseStrArg(args []string, i int, target *string) int {
	if i+1 < len(args) {
		i++
		*target = args[i]
	}
	return i
}

func runVercel(args []string) error {
	flags := parseVercelFlags(args)
	vc := vercelConfig{
		BuildCmd: flags.BuildCmd,
		GoFlags:  flags.GoFlags,
	}

	if _, err := os.Stat(flags.ConfigPath); err == nil {
		svcCfg, loadErr := runtime.LoadConfig(flags.ConfigPath)
		if loadErr != nil {
			return fmt.Errorf("load config: %w", loadErr)
		}
		if svcCfg.Deploy == nil || svcCfg.Deploy.Target == "" || svcCfg.Deploy.Target == "auto" {
			svcCfg.Deploy = &runtime.DeployConfig{Target: "vercel"}
		}
		if err := runtime.ValidateProjectStructure(flags.ConfigPath, "vercel"); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config: %w", err)
	}

	t := template.Must(template.New("vercel").Parse(tmplVercel))

	out := os.Stdout
	if flags.OutputPath != "" {
		f, createErr := os.Create(flags.OutputPath)
		if createErr != nil {
			return fmt.Errorf("create output: %w", createErr)
		}
		defer func() { if cerr := f.Close(); cerr != nil { logx.Errorf("close output: %v", cerr) } }()
		out = f
	}

	return t.Execute(out, vc)
}
