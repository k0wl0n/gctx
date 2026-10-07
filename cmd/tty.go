package cmd

import (
	"os"

	"golang.org/x/term"
)

// terminalUsable reports whether stdin and stdout are terminals and TERM can
// drive a full-screen UI.
func terminalUsable(getenv func(string) string, stdinTTY, stdoutTTY bool) bool {
	return stdinTTY && stdoutTTY && getenv("TERM") != "dumb"
}

// shouldLaunchTUI decides whether a bare `gctx` opens the TUI.
func shouldLaunchTUI(getenv func(string) string, stdinTTY, stdoutTTY bool) bool {
	if !terminalUsable(getenv, stdinTTY, stdoutTTY) {
		return false
	}
	switch getenv("CI") {
	case "", "false", "0":
	default:
		return false
	}
	return getenv("GCTX_NO_TUI") == ""
}

func stdioIsTerminal() (stdin, stdout bool) {
	return term.IsTerminal(int(os.Stdin.Fd())), term.IsTerminal(int(os.Stdout.Fd()))
}
