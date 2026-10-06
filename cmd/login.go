package cmd

import (
	"context"
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login <account-name>",
	Short: "Re-authenticate an existing account",
	Long: `Run the authentication flow (gcloud auth login + application-default login)
for an existing account and update the saved credentials.`,
	Example: `  # Re-authenticate 'my-account'
  gctx login my-account`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return runLogin(cmd.Context(), m, args[0])
	},
}

func runLogin(ctx context.Context, m *manager.Manager, name string) error {
	result, err := m.Login(ctx, name, present.LoginIO(os.Stdin, os.Stdout, os.Stderr))
	present.LoginDone(os.Stdout, os.Stderr, result, err)
	return err
}
