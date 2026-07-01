package main

import (
	_ "embed"
	"fmt"
	"os"
	"text/template"
)

//go:embed templates/deployment.tmpl
var tmplDeploy string

type kubeConfig struct {
	Name       string
	Namespace  string
	Image      string
	Port       int
	Replicas   int
	ReqCPU     string
	ReqMem     string
	LimitCPU   string
	LimitMem   string
	Secret     string
}

func runKube(args []string) error {
	cfg := kubeConfig{
		Namespace: "default",
		Port:      8080,
		Replicas:  3,
		ReqCPU:    "100m",
		ReqMem:    "64Mi",
		LimitCPU:  "500m",
		LimitMem:  "256Mi",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) { i++; cfg.Name = args[i] }
		case "--namespace":
			if i+1 < len(args) { i++; cfg.Namespace = args[i] }
		case "--image":
			if i+1 < len(args) { i++; cfg.Image = args[i] }
		case "--port":
			if i+1 < len(args) { i++; _, _ = fmt.Sscanf(args[i], "%d", &cfg.Port) }
		case "--replicas":
			if i+1 < len(args) { i++; _, _ = fmt.Sscanf(args[i], "%d", &cfg.Replicas) }
		}
	}

	if cfg.Name == "" { return fmt.Errorf("--name is required") }
	if cfg.Image == "" { return fmt.Errorf("--image is required") }

	t := template.Must(template.New("deploy").Parse(tmplDeploy))
	return t.Execute(os.Stdout, cfg)
}
