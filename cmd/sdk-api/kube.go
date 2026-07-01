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
			if i+1 < len(args) { i++; if _, err := fmt.Sscanf(args[i], "%d", &cfg.Port); err != nil {
			fmt.Fprintf(os.Stderr, "kube: parse port error: %v\n", err)
		} }
		case "--replicas":
			if i+1 < len(args) { i++; if _, err := fmt.Sscanf(args[i], "%d", &cfg.Replicas); err != nil {
			fmt.Fprintf(os.Stderr, "kube: parse replicas error: %v\n", err)
		} }
		}
	}

	if cfg.Name == "" { return fmt.Errorf("--name is required") }
	if cfg.Image == "" { return fmt.Errorf("--image is required") }

	t := template.Must(template.New("deploy").Parse(tmplDeploy))
	return t.Execute(os.Stdout, cfg)
}
