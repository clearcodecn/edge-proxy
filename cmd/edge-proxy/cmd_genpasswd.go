package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

func genPasswdCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gen-passwd <plain>",
		Short: "Generate a bcrypt hash for the admin password",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hash, err := bcrypt.GenerateFromPassword([]byte(args[0]), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("bcrypt: %w", err)
			}
			fmt.Println(string(hash))
			return nil
		},
	}
}
