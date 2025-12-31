package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured accounts",
	Example: `  # List all accounts
  gctx list`,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.ListAccounts()
	},
}
