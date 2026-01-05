package cmd

import (
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell [account-name]",
	Short: "Start a new shell with specific account context",
	Long: `Start a new shell session where gcloud commands run with the specified account.
This session is isolated and does not affect other terminals.
If no account is specified, an interactive selection menu is shown.`,
	Example: `  # Start shell for 'my-account'
  gctx shell my-account

  # Select account interactively
  gctx shell`,
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

		return m.StartShell(targetAccount)
	},
}
