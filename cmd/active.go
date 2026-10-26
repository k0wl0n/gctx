package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/spf13/cobra"
)

var activeCmd = &cobra.Command{
	Use:   "active [account-name]",
	Short: "Show or set currently active account",
	Example: `  # Show the currently active account
  gctx active

  # Switch to 'my-account' (same as gctx switch)
  gctx active my-account`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		// If an argument is provided, behave like switch
		if len(args) > 0 {
			return runSwitch(cmd.Context(), m, args[0])
		}

		// A lone gcp slot keeps the single-line format.
		slots := m.ActiveSlots()
		_, onlyGCP := slots[string(provider.GCP)]
		if len(slots) > 1 || (len(slots) == 1 && !onlyGCP) {
			present.ActiveSlots(os.Stdout, slots)
			return nil
		}
		present.Active(os.Stdout, m.ActiveAccount())
		return nil
	},
}
