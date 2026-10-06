package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/spf13/cobra"
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

		overview, err := m.Overview()
		if err != nil {
			return err
		}
		present.List(os.Stdout, overview)
		return nil
	},
}
