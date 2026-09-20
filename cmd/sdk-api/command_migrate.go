package main

import (
	"context"
	"fmt"
	"os"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply versioned SQL migrations",
	Long: `Applies versioned SQL migrations from a directory to a database.

Files are named <version>_<name>.sql (e.g. 0001_init.sql) and run in version
order. A schema_migrations table records what has been applied, so a migration
runs exactly once per environment and dev, staging and prod converge.

The connection comes from flags (--driver + --dsn) or from a service.yaml
database (--service + --db, defaulting to the first database).`,
}

var (
	migrateDir     string
	migrateDriver  string
	migrateDSN     string
	migrateToken   string
	migrateService string
	migrateDB      string
)

func openMigrateDB() (*db.Migrator, func(), error) {
	chosenDriver, chosenDSN, chosenToken := migrateDriver, migrateDSN, migrateToken
	if chosenDriver == "" || chosenDSN == "" {
		path := migrateService
		if path == "" {
			path = "service.yaml"
		}
		cfg, err := runtime.LoadConfig(path)
		if err != nil {
			return nil, nil, fmt.Errorf("load %s: %w", path, err)
		}
		if len(cfg.Databases) == 0 {
			return nil, nil, fmt.Errorf("no databases declared in %s", path)
		}
		chosen := cfg.Databases[0]
		if migrateDB != "" {
			found := false
			for _, d := range cfg.Databases {
				if d.Name == migrateDB {
					chosen, found = d, true
					break
				}
			}
			if !found {
				return nil, nil, fmt.Errorf("database %q not found in %s", migrateDB, path)
			}
		}
		chosenDriver, chosenDSN = chosen.Driver, os.ExpandEnv(chosen.URL)
		if chosenToken == "" {
			chosenToken = os.ExpandEnv(chosen.AuthToken)
		}
	}
	conn, err := db.OpenForDriver(chosenDriver, chosenDSN, chosenToken)
	if err != nil {
		return nil, nil, err
	}
	mig, err := db.NewMigrator(conn, migrateDir)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return mig, func() { _ = mig.Close(); _ = conn.Close() }, nil
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show applied and pending migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		mig, closeFn, err := openMigrateDB()
		if err != nil {
			return err
		}
		defer closeFn()
		status, err := mig.Status(context.Background())
		if err != nil {
			return err
		}
		for _, s := range status {
			mark := "pending"
			if s.Applied {
				mark = "applied"
			}
			if s.Modified {
				mark = "MODIFIED"
			}
			fmt.Printf("%04d  %-10s %s\n", s.Version, mark, s.Name)
		}
		return nil
	},
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply every pending migration",
	RunE: func(cmd *cobra.Command, args []string) error {
		mig, closeFn, err := openMigrateDB()
		if err != nil {
			return err
		}
		defer closeFn()
		done, err := mig.Up(context.Background())
		if err != nil {
			return err
		}
		if len(done) == 0 {
			fmt.Println("no pending migrations")
			return nil
		}
		fmt.Printf("applied %d migration(s): %v\n", len(done), done)
		return nil
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back the last applied migration",
	RunE: func(cmd *cobra.Command, args []string) error {
		mig, closeFn, err := openMigrateDB()
		if err != nil {
			return err
		}
		defer closeFn()
		version, err := mig.Down(context.Background())
		if err != nil {
			return err
		}
		if version == 0 {
			fmt.Println("nothing to roll back")
			return nil
		}
		fmt.Printf("rolled back migration %d\n", version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateStatusCmd, migrateUpCmd, migrateDownCmd)

	migrateCmd.PersistentFlags().StringVar(&migrateDir, "dir", "migrations", "Directory holding the versioned .sql migrations")
	migrateCmd.PersistentFlags().StringVar(&migrateDriver, "driver", "", "Database driver (postgres, mysql, turso, turso-serverless, libsql)")
	migrateCmd.PersistentFlags().StringVar(&migrateDSN, "dsn", "", "Database connection string (overrides --service)")
	migrateCmd.PersistentFlags().StringVar(&migrateToken, "auth-token", "", "Auth token for remote Turso/libSQL drivers (overrides the yaml auth_token)")
	migrateCmd.PersistentFlags().StringVar(&migrateService, "service", "service.yaml", "service.yaml to read the database from")
	migrateCmd.PersistentFlags().StringVar(&migrateDB, "db", "", "Database name inside service.yaml (default: the first one)")
}
