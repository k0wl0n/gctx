package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var loginCmd = &cobra.Command{
	Use:   "login <account-name>",
	Short: "Re-authenticate an existing account",
	Long: `Run the authentication flow (gcloud auth login + application-default login)
for an existing account and update the saved credentials.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.Login(args[0])
	},
}
