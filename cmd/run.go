package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <account-name> <gcloud-args>...",
	Short: "Run a gcloud command with specific account",
	Example: `  # Run 'gcloud storage ls' as 'my-account'
  gctx run my-account storage ls

  # Run 'gcloud compute instances list' as 'dev-account'
  gctx run dev-account compute instances list

  # Run with flags
  gctx run my-account storage buckets list --limit=1`,
	Args:               cobra.MinimumNArgs(2),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// With DisableFlagParsing=true, we need to handle help flag manually if it's the only arg
		if len(args) == 0 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help")) {
			return cmd.Help()
		}

		m, err := manager.New()
		if err != nil {
			return err
		}

		child, err := m.RunCommand(args[0], args[1:])
		if err != nil {
			return err
		}
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		return child.Run()
	},
}
