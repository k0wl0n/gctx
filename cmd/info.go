package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var infoCmd = &cobra.Command{
	Use:   "info <account-name>",
	Short: "Show detailed account information",
	Example: `  # Show details for 'my-account'
  gctx info my-account`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.ShowAccountInfo(args[0])
	},
}
