package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var runCmd = &cobra.Command{
	Use:   "run <account-name> <gcloud-args>...",
	Short: "Run a gcloud command with specific account",
	Example: `  # Run 'gcloud storage ls' as 'my-account'
  gctx run my-account storage ls

  # Run 'gcloud compute instances list' as 'dev-account'
  gctx run dev-account compute instances list`,
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.RunWithAccount(args[0], args[1:])
	},
}
