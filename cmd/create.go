package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
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

		result, err := m.Create(cmd.Context(), args[0], args[1])
		if err != nil {
			return err
		}
		present.Create(os.Stdout, os.Stderr, result)
		return runLogin(cmd.Context(), m, args[0])
	},
}
