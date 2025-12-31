package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var switchCmd = &cobra.Command{
	Use:   "switch <account-name>",
	Short: "Switch to a different account",
	Example: `  # Switch to 'my-account'
  gctx switch my-account`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.SwitchAccount(args[0])
	},
}
