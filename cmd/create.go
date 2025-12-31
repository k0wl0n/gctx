package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var autoSave bool

var createCmd = &cobra.Command{
	Use:   "create <account-name> <project-id>",
	Short: "Create a new account configuration",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.CreateAccount(args[0], args[1], autoSave)
	},
}

func init() {
	createCmd.Flags().BoolVar(&autoSave, "auto-save", false,
		"Automatically run auth and save credentials")
}
