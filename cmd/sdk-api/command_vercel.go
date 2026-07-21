package main

import (
	"github.com/spf13/cobra"
)

var vercelCmd = &cobra.Command{
	Use:   "vercel",
	Short: "Generate vercel.json for Vercel deployment",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		allArgs := []string{}
		if config, _ := cmd.Flags().GetString("config"); config != "" {
			allArgs = append(allArgs, "--config", config)
		}
		if output, _ := cmd.Flags().GetString("output"); output != "" {
			allArgs = append(allArgs, "--output", output)
		}
		if build, _ := cmd.Flags().GetString("build-command"); build != "" {
			allArgs = append(allArgs, "--build-command", build)
		}
		if goFlags, _ := cmd.Flags().GetString("go-flags"); goFlags != "" {
			allArgs = append(allArgs, "--go-flags", goFlags)
		}
		return runVercel(allArgs)
	},
}

func init() {
	vercelCmd.Flags().String("config", "service.yaml", "Path to service.yaml")
	vercelCmd.Flags().String("output", "", "Output file (default: stdout)")
	vercelCmd.Flags().String("build-command", "", "Custom build command")
	vercelCmd.Flags().String("go-flags", "", "Extra GO_BUILD_FLAGS")
	rootCmd.AddCommand(vercelCmd)
}
