package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sdk-api",
	Short: "SDK-API — microservice SDK generator",
	Long: `SDK-API is a code generation tool for creating microservices
using the natuleadan/sdk-api framework.

It generates project scaffolding, Dockerfiles, Kubernetes manifests,
and client SDKs from simple YAML configurations.`,
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().String("output", "", "Output format (json, table, text)")
}
