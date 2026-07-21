package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate service.yaml configuration",
	Long: `Validates a service YAML configuration file against the SDK schema.
Checks required fields, references, and value ranges.
Defaults to service.yaml if no file is specified.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "service.yaml"
		if len(args) > 0 {
			path = args[0]
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}

		strict, _ := cmd.Flags().GetBool("strict")
		verbose, _ := cmd.Flags().GetBool("verbose")

		logx.Disable()
		cfg, err := runtime.LoadConfig(path)
		if err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}

		if verbose {
			fmt.Printf("service: %s\n", cfg.Name)
			fmt.Printf("port: %d\n", cfg.Port)
			fmt.Printf("databases: %d\n", len(cfg.Databases))
			fmt.Printf("entries: %d\n", len(cfg.Entry))
			fmt.Printf("exits: %d\n", len(cfg.Exit))
			fmt.Printf("cron: %d\n", len(cfg.Cron))
			fmt.Printf("streams: %d\n", len(cfg.Stream))
		}

		var warnings []string
		if cfg.Name == "" {
			warnings = append(warnings, "service name is empty")
		}
		if cfg.Port <= 0 || cfg.Port > 65535 {
			warnings = append(warnings, fmt.Sprintf("invalid port: %d", cfg.Port))
		}
		for i, db := range cfg.Databases {
			if db.Driver == "postgres" && db.URL == "" {
				warnings = append(warnings, fmt.Sprintf("databases[%d] (%s): postgres url is empty", i, db.Name))
			}
		}
		for i, entry := range cfg.Entry {
			if entry.Path == "" {
				warnings = append(warnings, fmt.Sprintf("entry[%d]: path is empty", i))
			}
			if entry.APIVersion != "" && !regexp.MustCompile(`^v\d+$`).MatchString(entry.APIVersion) {
				warnings = append(warnings, fmt.Sprintf("entry[%d] (%s): invalid api_version %q", i, entry.Path, entry.APIVersion))
			}
		}

		if len(warnings) == 0 {
			fmt.Println("✓ configuration is valid")
			return nil
		}

		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "⚠ %s\n", w)
		}
		if strict {
			return fmt.Errorf("validation failed with %d warnings", len(warnings))
		}
		fmt.Printf("✓ configuration is valid (%d warnings)\n", len(warnings))
		return nil
	},
}

func init() {
	validateCmd.Flags().Bool("strict", false, "Fail on warnings")
	validateCmd.Flags().Bool("verbose", false, "Show all checks")
	rootCmd.AddCommand(validateCmd)
}
