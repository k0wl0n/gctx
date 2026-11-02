package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login <account-name>",
	Short: "Re-authenticate an existing account",
	Long: `Run the authentication flow (gcloud auth login + application-default login)
for an existing account and update the saved credentials.

If the application-default login fails (for example gcloud's
"(missing_code) Missing code parameter in response"), an interactive terminal
offers to retry only that step. Close old Google sign-in tabs before retrying.`,
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
		lio.RetryADC = adcRetryPrompt(os.Stdin, os.Stdout)
	}
	result, err := m.Login(ctx, name, lio)
	present.LoginDone(os.Stdout, os.Stderr, result, err)
	return err
}

// adcRetryPrompt explains a failed ADC login and asks whether to retry only
// that step. An empty answer means yes.
func adcRetryPrompt(in io.Reader, out io.Writer) func(error) bool {
	reader := bufio.NewReader(in)
	return func(error) bool {
		fmt.Fprintln(out, "\nThe browser callback did not return a code, so ADC login failed.")
		fmt.Fprintln(out, "Close any old Google sign-in tabs, then retry; the first login step is kept.")
		fmt.Fprint(out, "Retry ADC login only? [Y/n] ")
		answer, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "", "y", "yes":
			return true
		}
		return false
	}
}
