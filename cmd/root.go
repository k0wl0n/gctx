package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "gctx",
	Short: "Manage multiple GCP accounts seamlessly",
	Long: `gctx is a CLI tool to manage multiple GCP accounts with
automatic switching of both gcloud configurations and ADC credentials.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(saveCmd)
	rootCmd.AddCommand(switchCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(activeCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate completion script",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		}
		return fmt.Errorf("unsupported shell: %s", args[0])
	},
}
