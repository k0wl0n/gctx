package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var activeCmd = &cobra.Command{
	Use:   "active",
	Short: "Show currently active account",
	Example: `  # Show the currently active account
  gctx active`,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		active, err := m.GetActiveAccount()
		if err != nil {
			return err
		}

		fmt.Printf("Active account: %s\n", active)
		return nil
	},
}
