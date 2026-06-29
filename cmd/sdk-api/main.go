package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Printf("sdk-api version %s\n", version)
	case "new":
		if err := runNew(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "docker":
		if err := runDocker(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "kube":
		if err := runKube(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "client":
		if err := runClient(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`sdk-api — microservice SDK generator

Commands:
  sdk-api version              Show version
  sdk-api new <name> [flags]   Generate a new microservice
  sdk-api docker [flags]          Generate Dockerfile
  sdk-api kube [flags]            Generate Kubernetes deployment YAML
  sdk-api client [flags]          Generate client SDK (TypeScript, Python)

Flags for "new":
  --model string       Model name (default: the <name>)
  --fields strings     Field definitions: "name:string,price:float64,stock:int"
  --port int           HTTP port (default: 8080)
  --consume strings    NATS consumers: "stream:durable:handler"
  --publish strings    NATS producers: "stream:after_event"
  --dir string         Output directory (default: <name>)

Flags for "docker":
  --main string        Main file path (default: main.go)
  --name string        Binary name (default: service)
  --port int           Exposed port (default: 8080)
  --base string        Base image (default: scratch)

Flags for "kube":
  --name string        Service name (required)
  --image string       Container image (required)
  --namespace string   K8s namespace (default: default)
  --port int           Container port (default: 8080)
  --replicas int       Replicas (default: 3)

Flags for "client":
  --model string       Model name (required)
  --fields string      Field definitions: "name:string,price:float64"
  --lang string        Target language: ts, py, dart, java, kotlin (default: ts)
  --output string      Output file (default: stdout)

Examples:
  sdk-api version
  sdk-api new products-svc --model Product --fields "name:string,price:float64"
  sdk-api docker --name products-svc --port 8080
  sdk-api kube --name products-svc --image products:v1 --port 8080
  sdk-api client --model Product --fields "name:string,price:float64" --lang ts --output ./sdk.ts`)
}
