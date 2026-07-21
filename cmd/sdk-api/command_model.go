package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Generate Go model from SQL DDL",
	Long: `Generates a Go struct from a SQL CREATE TABLE statement.
Reads from a file or stdin.

Examples:
  sdk-api model from-sql schema.sql
  cat schema.sql | sdk-api model from-sql -
  sdk-api model from-sql --table products`,
}

var modelFromMongoCmd = &cobra.Command{
	Use:   "from-mongo",
	Short: "Generate Go struct from MongoDB collection",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		uri, _ := cmd.Flags().GetString("uri")
		db, _ := cmd.Flags().GetString("db")
		coll, _ := cmd.Flags().GetString("collection")
		if uri == "" || db == "" || coll == "" {
			return fmt.Errorf("--uri, --db, and --collection are required")
		}
		return runModelFromMongo(uri, db, coll)
	},
}

var modelFromSQLCmd = &cobra.Command{
	Use:   "from-sql [file]",
	Short: "Generate Go struct from SQL CREATE TABLE",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || args[0] == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			return runModelFromSQL(string(data))
		}
		return runModelFromSQLFile(args[0])
	},
}

func init() {
	rootCmd.AddCommand(modelCmd)
	modelCmd.AddCommand(modelFromSQLCmd)
	modelCmd.AddCommand(modelFromMongoCmd)
	modelFromMongoCmd.Flags().String("uri", "", "MongoDB URI (required)")
	modelFromMongoCmd.Flags().String("db", "", "Database name (required)")
	modelFromMongoCmd.Flags().String("collection", "", "Collection name (required)")
}
