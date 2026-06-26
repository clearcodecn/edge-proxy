package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const systemdUnit = `[Unit]
Description=edge-proxy reverse proxy admin
After=network.target nginx.service

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/edge-proxy run
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

const systemdUnitPath = "/etc/systemd/system/edge-proxy.service"

func installSystemdCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install-systemd",
		Short: "Write /etc/systemd/system/edge-proxy.service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := os.WriteFile(systemdUnitPath, []byte(systemdUnit), 0644); err != nil {
				return fmt.Errorf("write unit: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote %s\nRun:\n  systemctl daemon-reload\n  systemctl enable --now edge-proxy\n", systemdUnitPath)
			return nil
		},
	}
}
