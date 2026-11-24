package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/k0wl0n/gctx/pkg/debuglog"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login <account-name>",
	Short: "Re-authenticate an existing account",
	Long: `Run the authentication flow (gcloud auth login + application-default login)
for an existing account and update the saved credentials.

If a browser step fails because gcloud's callback on localhost:8085 got no
code ("(missing_code) Missing code parameter in response"), an interactive
terminal offers to retry only that step. Close old Google sign-in tabs before
retrying. Set GCTX_DEBUG=1 to write redacted diagnostics to the debug log.`,
	Example: `  # Re-authenticate 'my-account'
  gctx login my-account`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := manager.New()
		if err != nil {
			return err
		}

		return runLogin(cmd.Context(), m, args[0])
	},
}

func runLogin(ctx context.Context, m *manager.Manager, name string) error {
	lio := present.LoginIO(os.Stdin, os.Stdout, os.Stderr)
	if stdinTTY, _ := stdioIsTerminal(); stdinTTY {
		lio.RetryStep = present.RetryPrompt(os.Stdin, os.Stdout)
	}
	result, err := m.Login(ctx, name, lio)
	present.LoginDone(os.Stdout, os.Stderr, result, err)
	if errors.Is(err, provider.ErrBrowserCallback) && debuglog.EnabledByEnv() {
		if path, pathErr := debuglog.Path(); pathErr == nil {
			fmt.Fprintf(os.Stderr, "debug log: %s\n", path)
		}
	}
	return err
}
