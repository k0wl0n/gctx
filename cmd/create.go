package cmd

import (
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <account-name> <project-id>",
	Short: "Create a new account configuration",
	Example: `  # Create a new account (auto-authenticates)
  gctx create my-account my-project-id`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.CreateAccount(args[0], args[1])
	},
}
