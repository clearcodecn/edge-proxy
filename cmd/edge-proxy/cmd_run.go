package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

const defaultConfigPath = "/etc/edge-proxy/config.yaml"

func runCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the edge-proxy main process (HTTP admin + crons)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("TODO: load %s and start main loop\n", configPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", defaultConfigPath, "path to config.yaml")
	return cmd
}
