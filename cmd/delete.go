package cmd

import (
	"fmt"
	"os"

	"github.com/k0wl0n/gctx/pkg/config"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/spf13/cobra"
)

var (
	deleteGcloudConfig bool
	deleteForce        bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete <account-name>",
	Short: "Delete an account",
	Example: `  # Delete an account (keeps gcloud config)
  gctx delete my-account

  # Delete an account AND its gcloud configuration
  gctx delete my-account --gcloud-config`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		if deleteGcloudConfig && !deleteForce {
			if err := refuseUserOwnedConfig(m, args[0]); err != nil {
				return err
			}
		}
		result, err := m.Delete(cmd.Context(), args[0], manager.DeleteOptions{GcloudConfig: deleteGcloudConfig, RemoveDir: true})
		if err != nil {
			return err
		}
		present.Delete(os.Stdout, os.Stderr, result)
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteGcloudConfig, "gcloud-config", false,
		"Also delete gcloud configuration")
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false,
		"With --gcloud-config, also delete a configuration that existed before gctx")
}

// refuseUserOwnedConfig stops --gcloud-config from deleting a gcloud
// configuration that was imported or reused rather than created by gctx.
func refuseUserOwnedConfig(m *manager.Manager, name string) error {
	info, err := m.Info(name)
	if err != nil {
		return err
	}
	switch info.Settings[provider.SettingConfigOrigin] {
	case provider.ConfigOriginImported, provider.ConfigOriginExisting:
		return fmt.Errorf("gcloud configuration %s existed before %s; add --force to delete it as well", info.ConfigName, config.AppName)
	}
	return nil
}
