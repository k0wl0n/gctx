package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <account-name>",
	Short: "Show detailed account information",
	Example: `  # Show details for 'my-account'
  gctx info my-account`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		status, err := m.Info(args[0])
		if err != nil {
			return err
		}
		present.Info(os.Stdout, status)
		return nil
	},
}
