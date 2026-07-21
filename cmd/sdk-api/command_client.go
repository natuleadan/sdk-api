package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Generate client SDK (TypeScript, Python, Dart, Java, Kotlin)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		model, _ := cmd.Flags().GetString("model")
		fields, _ := cmd.Flags().GetString("fields")
		if model == "" || fields == "" {
			return fmt.Errorf("--model and --fields are required")
		}
		allArgs := []string{"--model", model, "--fields", fields}
		if lang, _ := cmd.Flags().GetString("lang"); lang != "" {
			allArgs = append(allArgs, "--lang", lang)
		}
		if output, _ := cmd.Flags().GetString("output"); output != "" {
			allArgs = append(allArgs, "--output", output)
		}
		return runClient(allArgs)
	},
}

func init() {
	clientCmd.Flags().String("model", "", "Model name (required)")
	clientCmd.Flags().String("fields", "", "Field definitions: \"name:string,price:float64\" (required)")
	clientCmd.Flags().String("lang", "ts", "Target language: ts, py, dart, java, kotlin")
	clientCmd.Flags().String("output", "", "Output file (default: stdout)")
	rootCmd.AddCommand(clientCmd)
}
