package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Generate a new microservice",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reconstruct args to pass to runNew
		allArgs := args
		if model, _ := cmd.Flags().GetString("model"); model != "" {
			allArgs = append(allArgs, "--model", model)
		}
		if fields, _ := cmd.Flags().GetString("fields"); fields != "" {
			allArgs = append(allArgs, "--fields", fields)
		}
		if port, _ := cmd.Flags().GetInt("port"); port != 8080 {
			allArgs = append(allArgs, "--port", fmt.Sprintf("%d", port))
		}
		if consume, _ := cmd.Flags().GetString("consume"); consume != "" {
			allArgs = append(allArgs, "--consume", consume)
		}
		if publish, _ := cmd.Flags().GetString("publish"); publish != "" {
			allArgs = append(allArgs, "--publish", publish)
		}
		if exitW, _ := cmd.Flags().GetString("exit"); exitW != "" {
			allArgs = append(allArgs, "--exit", exitW)
		}
		if cron, _ := cmd.Flags().GetString("cron"); cron != "" {
			allArgs = append(allArgs, "--cron", cron)
		}
		if grpc, _ := cmd.Flags().GetBool("grpc"); grpc {
			allArgs = append(allArgs, "--grpc")
		}
		if withTests, _ := cmd.Flags().GetBool("with-tests"); withTests {
			allArgs = append(allArgs, "--with-tests")
		}
		if rests, _ := cmd.Flags().GetStringArray("rest"); len(rests) > 0 {
			for _, r := range rests {
				allArgs = append(allArgs, "--rest", r)
			}
		}
		if split, _ := cmd.Flags().GetBool("split"); split {
			allArgs = append(allArgs, "--split")
		}
		if grpcPort, _ := cmd.Flags().GetInt("grpc-port"); grpcPort > 0 {
			allArgs = append(allArgs, "--grpc-port", fmt.Sprintf("%d", grpcPort))
		}
		if dir, _ := cmd.Flags().GetString("dir"); dir != "" {
			allArgs = append(allArgs, "--dir", dir)
		}
		return runNew(allArgs)
	},
}

func init() {
	newCmd.Flags().String("model", "", "Model name")
	newCmd.Flags().String("fields", "", "Field definitions: \"name:string,price:float64\"")
	newCmd.Flags().Int("port", 8080, "HTTP port")
	newCmd.Flags().String("consume", "", "NATS consumers: \"stream:durable:handler\"")
	newCmd.Flags().String("publish", "", "NATS producers: \"stream:after_event\"")
	newCmd.Flags().String("exit", "", "Exit workers: \"stream:handler:name\"")
	newCmd.Flags().String("cron", "", "Cron jobs: \"handler:name\"")
	newCmd.Flags().Bool("grpc", false, "Enable gRPC server generation")
	newCmd.Flags().Int("grpc-port", 0, "gRPC server port (default: HTTP port + 1)")
	newCmd.Flags().Bool("with-tests", false, "Generate test stubs")
	newCmd.Flags().StringArray("rest", []string{}, "REST endpoints: \"GROUP|METHOD:/path:Handler\"")
	newCmd.Flags().Bool("split", false, "Generate one handler file per endpoint")
	newCmd.Flags().String("dir", "", "Output directory")
	rootCmd.AddCommand(newCmd)
}
