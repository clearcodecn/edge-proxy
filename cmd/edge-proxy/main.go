package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "edge-proxy",
		Short: "Self-hosted reverse proxy edge node with automatic Let's Encrypt SSL",
	}
	root.AddCommand(
		runCmd(),
		initCmd(),
		genPasswdCmd(),
		installSystemdCmd(),
		versionCmd(),
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
