package main

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and git commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			info, ok := debug.ReadBuildInfo()
			if !ok {
				fmt.Println("unknown")
				return nil
			}
			rev := "unknown"
			dirty := ""
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					rev = s.Value
				}
				if s.Key == "vcs.modified" && s.Value == "true" {
					dirty = "-dirty"
				}
			}
			fmt.Printf("edge-proxy %s%s (go %s)\n", rev, dirty, info.GoVersion)
			return nil
		},
	}
}
