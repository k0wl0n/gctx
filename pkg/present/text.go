// Package present renders manager results as the CLI's text output. It is the
// only place those strings live, so the CLI and the suspended TUI login flow
// print identical text.
package present

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
)

// List prints `gctx list`.
func List(w io.Writer, overview manager.Overview) {
	if len(overview.Accounts) == 0 {
		fmt.Fprintln(w, "No accounts configured")
		return
	}

	fmt.Fprintln(w, "\nConfigured Accounts:")
	fmt.Fprintln(w, "===================")
	for _, account := range overview.Accounts {
		active := ""
		if account.Active {
			active = " ← active"
		}
		email := ""
		if account.Email != "" {
			email = fmt.Sprintf(" [%s]", account.Email)
		}
		fmt.Fprintf(w, "  %s (%s)%s%s\n", account.Name, account.ProjectID, email, active)
	}
}

// Active prints `gctx active`.
func Active(w io.Writer, name string) {
	fmt.Fprintf(w, "Active account: %s\n", name)
}

// Info prints `gctx info`.
func Info(w io.Writer, status manager.AccountStatus) {
	fmt.Fprintf(w, "\nAccount: %s\n", status.Name)
	fmt.Fprintln(w, strings.Repeat("=", 50))
	fmt.Fprintf(w, "Project ID:       %s\n", status.ProjectID)
	fmt.Fprintf(w, "Config Name:      %s\n", status.ConfigName)
	if status.Email != "" {
		fmt.Fprintf(w, "Email:            %s\n", status.Email)
	}
	if status.ADCPath != "" {
		fmt.Fprintf(w, "ADC Path:         %s\n", status.ADCPath)
		if !status.RecordedADCModTime.IsZero() {
			fmt.Fprintf(w, "ADC Last Modified: %s\n", status.RecordedADCModTime.Format("2006-01-02 15:04:05"))
		}
	}
	fmt.Fprintf(w, "Created:          %s\n", status.CreatedAt.Format("2006-01-02 15:04:05"))
	if status.Active {
		fmt.Fprintln(w, "\nThis is the active account.")
	}
}

// Create prints the outcome of the configuration steps of `gctx create`.
// New diagnostics go to errw.
func Create(w, errw io.Writer, result manager.CreateResult) {
	for _, warning := range result.Warnings {
		if warning.Code == manager.WarnConfigReused {
			fmt.Fprintf(errw, "Warning: %s\n", warning.Message)
		}
	}
	fmt.Fprintf(w, "Created gcloud configuration: %s\n", result.ConfigName)
	if result.ProjectSet {
		fmt.Fprintf(w, "Set project: %s\n", result.ProjectID)
	}
	for _, warning := range result.Warnings {
		if warning.Code == manager.WarnSetProjectFailed {
			fmt.Fprintf(w, "Warning: Failed to set project ID (%s). Proceeding to authentication...\n", warning.Message)
		}
	}
	fmt.Fprintf(w, "Account '%s' added to configuration.\n\n", result.Account)
}

// LoginIO connects the manager's login flow to a terminal and prints its
// progress lines.
func LoginIO(stdin io.Reader, stdout, stderr io.Writer) manager.LoginIO {
	return manager.LoginIO{Stdin: stdin, Stdout: stdout, Stderr: stderr, Progress: LoginProgress(stdout)}
}

// LoginProgress prints one line per login event.
func LoginProgress(w io.Writer) func(manager.LoginEvent) {
	return func(event manager.LoginEvent) {
		switch event.Kind {
		case manager.LoginAuthStarted:
			fmt.Fprintln(w, "Running authentication...")
		case manager.LoginAuthDone:
			fmt.Fprintln(w, "Logged in successfully.")
		case manager.LoginADCStarted:
			fmt.Fprintln(w, "Running ADC authentication...")
		case manager.LoginSetProjectFailed:
			fmt.Fprintf(w, "Warning: failed to set project ID after login: %s\n", event.Message)
		case manager.LoginVerifyingADC:
			fmt.Fprintln(w, "Watching for ADC file changes...")
		}
	}
}

// LoginDone prints the closing text of the login flow. err is the error the
// flow returned, which the caller still returns itself.
func LoginDone(w, errw io.Writer, result manager.LoginResult, err error) {
	if err != nil {
		var stepErr *gcloud.StepError
		if errors.As(err, &stepErr) && stepErr.Step == manager.StepADCLogin {
			fmt.Fprintf(w, "\n⚠️  ADC authentication failed or was interrupted.\n")
			fmt.Fprintf(w, "This is usually because:\n")
			fmt.Fprintf(w, "  - You cancelled the authentication (Ctrl+C)\n")
			fmt.Fprintf(w, "  - The browser authentication was not completed\n")
			fmt.Fprintf(w, "  - Network connectivity issues\n\n")
			fmt.Fprintf(w, "To complete setup, run: gctx login %s\n", result.Account)
		}
		return
	}
	for _, warning := range result.Warnings {
		if warning.Code == manager.WarnADCNotRefreshed {
			fmt.Fprintf(errw, "Warning: %s\n", warning.Message)
		}
	}
	fmt.Fprintf(w, "ADC credentials auto-saved for: %s\n", result.Account)
	fmt.Fprintf(w, "Saved to: %s\n\n", result.ADCPath)
	fmt.Fprintf(w, "Account '%s' is ready to use!\n", result.Account)
	fmt.Fprintf(w, "Run: gctx switch %s\n", result.Account)
}

// Switch prints the outcome of `gctx switch`.
func Switch(w, errw io.Writer, result manager.SwitchResult) {
	for _, warning := range result.Warnings {
		switch warning.Code {
		case manager.WarnADCMissing:
			fmt.Fprintf(w, "\nWarning: No saved ADC credentials for account '%s'.\n", result.Account)
			fmt.Fprintf(w, "Run 'gctx login %s' to authenticate and save credentials.\n\n", result.Account)
		case manager.WarnReauthRequired:
			fmt.Fprintf(w, "\nWarning: Failed to set project ID because re-authentication is required.\n")
			fmt.Fprintf(w, "Please run: gctx login %s\n\n", result.Account)
		case manager.WarnSetProjectFailed:
			fmt.Fprintf(w, "Warning: failed to set project ID: %s\n", warning.Message)
		default:
			fmt.Fprintf(errw, "Warning: %s\n", warning.Message)
		}
	}
	fmt.Fprintf(w, "Switched to account: %s (%s)\n", result.Account, result.ProjectID)
}

// Save prints the outcome of `gctx save`.
func Save(w io.Writer, result manager.SaveResult) {
	fmt.Fprintf(w, "ADC credentials saved for: %s\n", result.Account)
	fmt.Fprintf(w, "Location: %s\n", result.ADCPath)
}

// Delete prints the outcome of `gctx delete`.
func Delete(w io.Writer, result manager.DeleteResult) {
	fmt.Fprintf(w, "Deleted account: %s\n", result.Account)
}

// ShellBanner prints the text shown before a shell starts.
func ShellBanner(w io.Writer, name string) {
	fmt.Fprintf(w, "Starting shell for account '%s'...\n", name)
	fmt.Fprintln(w, "Type 'exit' to return to the previous context.")
}
