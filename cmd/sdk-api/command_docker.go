package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Generate Dockerfile",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		allArgs := []string{}
		if main, _ := cmd.Flags().GetString("main"); main != "" {
			allArgs = append(allArgs, "--main", main)
		}
		if name, _ := cmd.Flags().GetString("name"); name != "" {
			allArgs = append(allArgs, "--name", name)
		}
		if port, _ := cmd.Flags().GetInt("port"); port != 8080 {
			allArgs = append(allArgs, "--port", fmt.Sprintf("%d", port))
		}
		if base, _ := cmd.Flags().GetString("base"); base != "" {
			allArgs = append(allArgs, "--base", base)
		}
		return runDocker(allArgs)
	},
}

func init() {
	dockerCmd.Flags().String("main", "main.go", "Main file path")
	dockerCmd.Flags().String("name", "service", "Binary name")
	dockerCmd.Flags().Int("port", 8080, "Exposed port")
	dockerCmd.Flags().String("base", "scratch", "Base image")
	rootCmd.AddCommand(dockerCmd)
}
