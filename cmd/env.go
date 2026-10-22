package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env <account-name>",
	Short: "Print shell exports that select an account",
	Example: `  # Apply an account to the current shell
  eval "$(gctx env my-account)"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		entries, err := m.Env(args[0])
		if err != nil {
			return err
		}
		present.Env(os.Stdout, entries)
		return nil
	},
}
