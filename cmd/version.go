package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// These variables are set via ldflags during build
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of gctx",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gctx version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built at: %s\n", date)
		fmt.Printf("go version: %s\n", runtime.Version())
		fmt.Printf("os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
