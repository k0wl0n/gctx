package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var saveCmd = &cobra.Command{
	Use:   "save <account-name>",
	Short: "Save current ADC credentials for an account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.SaveCredentials(args[0])
	},
}
