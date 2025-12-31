package cmd

import (
	"fmt"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/spf13/cobra"
)

var activeCmd = &cobra.Command{
	Use:   "active [account-name]",
	Short: "Show or set currently active account",
	Example: `  # Show the currently active account
  gctx active

  # Switch to 'my-account' (same as gctx switch)
  gctx active my-account`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		// If an argument is provided, behave like switch
		if len(args) > 0 {
			return m.SwitchAccount(args[0])
		}

		// Otherwise, show active account
		active, err := m.GetActiveAccount()
		if err != nil {
			return err
		}

		fmt.Printf("Active account: %s\n", active)
		return nil
	},
}
