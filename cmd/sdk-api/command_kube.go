package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var kubeCmd = &cobra.Command{
	Use:   "kube",
	Short: "Generate Kubernetes deployment YAML",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		allArgs := []string{}
		if name, _ := cmd.Flags().GetString("name"); name != "" {
			allArgs = append(allArgs, "--name", name)
		}
		if image, _ := cmd.Flags().GetString("image"); image != "" {
			allArgs = append(allArgs, "--image", image)
		}
		if ns, _ := cmd.Flags().GetString("namespace"); ns != "" {
			allArgs = append(allArgs, "--namespace", ns)
		}
		if port, _ := cmd.Flags().GetInt("port"); port != 8080 {
			allArgs = append(allArgs, "--port", fmt.Sprintf("%d", port))
		}
		if replicas, _ := cmd.Flags().GetInt("replicas"); replicas != 3 {
			allArgs = append(allArgs, "--replicas", fmt.Sprintf("%d", replicas))
		}
		return runKube(allArgs)
	},
}

func init() {
	kubeCmd.Flags().String("name", "", "Service name (required)")
	kubeCmd.Flags().String("image", "", "Container image (required)")
	kubeCmd.Flags().String("namespace", "default", "K8s namespace")
	kubeCmd.Flags().Int("port", 8080, "Container port")
	kubeCmd.Flags().Int("replicas", 3, "Replicas")
	_ = kubeCmd.MarkFlagRequired("name")
	_ = kubeCmd.MarkFlagRequired("image")
	rootCmd.AddCommand(kubeCmd)
}
