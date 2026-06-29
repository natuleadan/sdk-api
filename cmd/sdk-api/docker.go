package main

import (
	_ "embed"
	"fmt"
	"os"
	"text/template"
)

//go:embed templates/docker.tmpl
var tmplDocker string

type dockerConfig struct {
	MainFile string
	ExeName  string
	Port     int
	Version  string
	Base     string
}

func runDocker(args []string) error {
	cfg := dockerConfig{
		MainFile: "main.go",
		ExeName:  "service",
		Port:     8080,
		Version:  "1.26",
		Base:     "scratch",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--main":
			if i+1 < len(args) { i++; cfg.MainFile = args[i] }
		case "--name":
			if i+1 < len(args) { i++; cfg.ExeName = args[i] }
		case "--port":
			if i+1 < len(args) { i++; fmt.Sscanf(args[i], "%d", &cfg.Port) }
		case "--base":
			if i+1 < len(args) { i++; cfg.Base = args[i] }
		}
	}

	t := template.Must(template.New("docker").Parse(tmplDocker))
	return t.Execute(os.Stdout, cfg)
}
