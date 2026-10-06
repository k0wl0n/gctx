package cmd

import (
	"context"
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch [account-name]",
	Short: "Switch to a different account",
	Example: `  # Switch to 'my-account'
  gctx switch my-account

  # Switch interactively (fuzzy search)
  gctx switch`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		targetAccount := ""
		if len(args) > 0 {
			targetAccount = args[0]
		} else {
			// Interactive mode
			selected, err := m.SelectAccountInteractive()
			if err != nil {
				return err
			}
			targetAccount = selected
		}

		return runSwitch(cmd.Context(), m, targetAccount)
	},
}

func runSwitch(ctx context.Context, m *manager.Manager, name string) error {
	result, err := m.Switch(ctx, name)
	if err != nil {
		return err
	}
	present.Switch(os.Stdout, os.Stderr, result)
	return nil
}
