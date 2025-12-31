package cmd

import (
	"github.com/spf13/cobra"
	"github.com/k0wl0n/gctx/pkg/manager"
)

var deleteGcloudConfig bool

var deleteCmd = &cobra.Command{
	Use:   "delete <account-name>",
	Short: "Delete an account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return m.DeleteAccount(args[0], deleteGcloudConfig)
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteGcloudConfig, "gcloud-config", false,
		"Also delete gcloud configuration")
}
