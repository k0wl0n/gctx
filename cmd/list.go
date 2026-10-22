package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/spf13/cobra"
)

var listProvider string

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured accounts",
	Example: `  # List all accounts
  gctx list

  # List only one provider's accounts
  gctx list --provider gcp`,
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		overview, err := m.Overview()
		if err != nil {
			return err
		}
		opts := present.ListOptions{Filter: provider.ID(listProvider)}
		if opts.Filter != "" {
			if err := checkRegistered(overview, opts.Filter); err != nil {
				return err
			}
		}
		present.List(os.Stdout, overview, opts)
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&listProvider, "provider", "", "show only accounts of this provider")
}

func checkRegistered(overview manager.Overview, id provider.ID) error {
	valid := make([]provider.ID, 0, len(overview.Providers))
	for _, info := range overview.Providers {
		if info.ID == id {
			return nil
		}
		valid = append(valid, info.ID)
	}
	return manager.NewUnknownProviderError(id, valid)
}
