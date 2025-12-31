package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var runCmd = &cobra.Command{
	Use:   "run <account-name> <gcloud-args>...",
	Short: "Run a gcloud command with specific account",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.RunWithAccount(args[0], args[1:])
	},
}
