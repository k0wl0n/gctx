package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/spf13/cobra"
)

var createProvider string

var createCmd = &cobra.Command{
	Use:   "create <account-name> <target>",
	Short: "Create a new account configuration",
	Example: `  # Create a new account (auto-authenticates)
  gctx create my-account my-project-id

  # Create an account for another provider
  gctx create prod prod-profile --provider aws`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		result, err := m.CreateAccount(cmd.Context(), manager.CreateRequest{
			Provider: provider.ID(createProvider), Name: args[0], Target: args[1],
		})
		if err != nil {
			return err
		}
		present.Create(os.Stdout, os.Stderr, result)
		return runLogin(cmd.Context(), m, args[0])
	},
}

func init() {
	createCmd.Flags().StringVar(&createProvider, "provider", string(provider.GCP), "cloud provider: gcp, aws, azure or tencent")
}
