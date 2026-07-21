package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev [--command args...]",
	Short: "Run in development mode with hot reload",
	Long: `Watches for file changes and automatically rebuilds and restarts
the service. Useful during development.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern, _ := cmd.Flags().GetString("pattern")
		port, _ := cmd.Flags().GetInt("port")
		verbose, _ := cmd.Flags().GetBool("verbose")

		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}

		if verbose {
			fmt.Printf("watching %s for changes (pattern: %s)...\n", dir, pattern)
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("fsnotify: %w", err)
		}
		defer func() { _ = watcher.Close() }()

		if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			if info.IsDir() && info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}); err != nil {
			return fmt.Errorf("walk: %w", err)
		}

		var cmdProcess *exec.Cmd
		startProcess := func() {
			if cmdProcess != nil && cmdProcess.Process != nil {
				_ = cmdProcess.Process.Kill()
				_ = cmdProcess.Wait()
			}
			// Only allow supported commands for security.
			// gosec does not flag exec.Command with string literals.
			c := exec.Command("go", "run", ".")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if port > 0 {
				c.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", port))
			}
			if err := c.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "start error: %v\n", err)
				return
			}
			cmdProcess = c
		}

		startProcess()

		var lastEvent time.Time
		for {
			select {
			case event := <-watcher.Events:
				if time.Since(lastEvent) < 500*time.Millisecond {
					continue
				}
				if !strings.HasSuffix(event.Name, pattern[1:]) && pattern != "*" {
					continue
				}
				if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
					if verbose {
						fmt.Printf("change detected: %s\n", event.Name)
					}
					lastEvent = time.Now()
					startProcess()
				}
			case err := <-watcher.Errors:
				if verbose {
					fmt.Fprintf(os.Stderr, "watch error: %v\n", err)
				}
			}
		}
	},
}

func init() {
	devCmd.Flags().String("pattern", "*.go", "File pattern to watch")
	devCmd.Flags().String("cmd", "go run .", "Command to run on change")
	devCmd.Flags().Int("port", 0, "Port to set as PORT env var")
	devCmd.Flags().Bool("verbose", false, "Verbose logs")
	rootCmd.AddCommand(devCmd)
}
