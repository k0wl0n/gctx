package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/tui"
	"github.com/spf13/cobra"
)

var tuiDebug bool

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive terminal UI",
	Long: `Open the full-screen terminal UI to browse accounts, switch, log in,
create, delete, run provider CLI commands and open isolated shells.`,
	Example: `  # Open the interactive UI
  gctx tui`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		stdinTTY, stdoutTTY := stdioIsTerminal()
		if !terminalUsable(os.Getenv, stdinTTY, stdoutTTY) {
			return errors.New(notTerminalMessage(runtime.GOOS, os.Getenv))
		}
		return runTUI(cmd.Context(), tuiDebug || os.Getenv("GCTX_DEBUG") == "1")
	},
}

func init() {
	tuiCmd.Flags().BoolVar(&tuiDebug, "debug", false, "Write a redacted debug log (same as GCTX_DEBUG=1)")
	rootCmd.AddCommand(tuiCmd)
}

func notTerminalMessage(goos string, getenv func(string) string) string {
	message := "gctx tui requires an interactive terminal (stdin and stdout must be a TTY)"
	if goos == "windows" && (getenv("MSYSTEM") != "" || getenv("TERM_PROGRAM") == "mintty") {
		message += "\nMinTTY (Git Bash) is not supported directly: run gctx from Windows Terminal, or prefix the command with winpty"
	}
	return message
}

func runTUI(ctx context.Context, debug bool) error {
	m, err := manager.New()
	if err != nil {
		return err
	}
	if err := tui.Run(ctx, m, tui.Options{Version: version, Debug: debug}); err != nil {
		return fmt.Errorf("%w\nRun 'gctx --help' for available commands.", err)
	}
	return nil
}
