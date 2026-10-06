package cmd

import (
	"os"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/spf13/cobra"
)

var saveCmd = &cobra.Command{
	Use:   "save <account-name>",
	Short: "Save current ADC credentials for an account",
	Example: `  # Save the current ADC file for 'my-account'
  # Useful if you ran 'gcloud auth application-default login' manually
  gctx save my-account`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		result, err := m.SaveCredentials(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		present.Save(os.Stdout, result)
		return nil
	},
}
