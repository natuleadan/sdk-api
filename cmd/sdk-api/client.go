package main

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
)

//go:embed templates/client_ts.tmpl
var clientTSTmpl string

//go:embed templates/client_py.tmpl
var clientPyTmpl string

//go:embed templates/client_dart.tmpl
var clientDartTmpl string

//go:embed templates/client_java.tmpl
var clientJavaTmpl string

//go:embed templates/client_kotlin.tmpl
var clientKotlinTmpl string

type clientField struct {
	Name string
	Type string
	JSON string
}

type clientModel struct {
	Name     string
	Resource string
	Fields   []clientField
}

func runClient(args []string) error {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	model := fs.String("model", "", "Model name (required)")
	fields := fs.String("fields", "", "Field definitions: \"name:string,price:float64\"")
	lang := fs.String("lang", "ts", "Target language: ts, py, dart, java, kotlin")
	output := fs.String("output", "", "Output file (default: stdout)")
	_ = fs.Parse(args)

	if *model == "" || *fields == "" {
		fmt.Fprintln(os.Stderr, "Usage: sdk-api client --model Product --fields \"name:string,price:float64\" --lang ts --output ./sdk.ts")
		fs.PrintDefaults()
		os.Exit(1)
	}

	var m clientModel
	m.Name = *model
	m.Resource = toSnake(plural(*model))
	for _, f := range strings.Split(*fields, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		parts := strings.SplitN(f, ":", 2)
		name := parts[0]
		goType := "string"
		if len(parts) > 1 {
			goType = parts[1]
		}
		m.Fields = append(m.Fields, clientField{
			Name: pascalCase(name),
			Type: clientType(goType, *lang),
			JSON: toSnake(name),
		})
	}

	var tmplStr string
	clientTemplates := map[string]string{
		"ts":     clientTSTmpl,
		"py":     clientPyTmpl,
		"dart":   clientDartTmpl,
		"java":   clientJavaTmpl,
		"kotlin": clientKotlinTmpl,
	}
	var ok bool
	if tmplStr, ok = clientTemplates[*lang]; !ok {
		return fmt.Errorf("unsupported language: %s (use ts, py, dart, java, or kotlin)", *lang)
	}

	tmpl, err := template.New("client").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}

	var w io.Writer
	if *output == "" || *output == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(*output)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		w = f
	}

	if err := tmpl.Execute(w, m); err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	return nil
}

func clientType(goType, lang string) string {
	switch lang {
	case "ts":
		return tsType(goType)
	case "py":
		return pyType(goType)
	case "dart":
		return dartType(goType)
	case "java":
		return javaType(goType)
	case "kotlin":
		return kotlinType(goType)
	}
	return "Any"
}

func tsType(goType string) string {
	switch goType {
	case "int", "int64", "float64", "float32":
		return "number"
	case "string":
		return "string"
	case "bool":
		return "boolean"
	case "time.Time":
		return "string"
	default:
		return "any"
	}
}

func pyType(goType string) string {
	switch goType {
	case "int", "int64":
		return "int"
	case "float64", "float32":
		return "float"
	case "string":
		return "str"
	case "bool":
		return "bool"
	case "time.Time":
		return "str"
	default:
		return "Any"
	}
}

func dartType(goType string) string {
	switch goType {
	case "int", "int64":
		return "int"
	case "float64", "float32":
		return "double"
	case "string":
		return "String"
	case "bool":
		return "bool"
	case "time.Time":
		return "String"
	default:
		return "dynamic"
	}
}

func javaType(goType string) string {
	switch goType {
	case "int":
		return "int"
	case "int64":
		return "long"
	case "float64":
		return "double"
	case "float32":
		return "float"
	case "string":
		return "String"
	case "bool":
		return "boolean"
	case "time.Time":
		return "String"
	default:
		return "String"
	}
}

func kotlinType(goType string) string {
	switch goType {
	case "int":
		return "Int"
	case "int64":
		return "Long"
	case "float64":
		return "Double"
	case "float32":
		return "Float"
	case "string":
		return "String"
	case "bool":
		return "Boolean"
	case "time.Time":
		return "String"
	default:
		return "String"
	}
}
