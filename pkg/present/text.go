// Package present renders manager results as the CLI's text output. It is the
// only place those strings live, so the CLI and the suspended TUI login flow
// print identical text.
package present

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// ListOptions refines `gctx list`.
type ListOptions struct {
	// Filter keeps only that provider's accounts and always shows the column.
	Filter provider.ID
}

// List prints `gctx list`. A PROVIDER column appears only when the rows span
// more than one provider or a filter was requested.
func List(w io.Writer, overview manager.Overview, options ...ListOptions) {
	var opts ListOptions
	if len(options) > 0 {
		opts = options[0]
	}
	if len(overview.Accounts) == 0 {
		fmt.Fprintln(w, "No accounts configured")
		return
	}

	showProvider := opts.Filter != ""
	for _, account := range overview.Accounts {
		if account.Provider != overview.Accounts[0].Provider {
			showProvider = true
		}
	}

	fmt.Fprintln(w, "\nConfigured Accounts:")
	fmt.Fprintln(w, "===================")
	if showProvider {
		fmt.Fprintf(w, "  %-*s  ACCOUNT\n", providerColumnWidth, "PROVIDER")
	}
	for _, account := range overview.Accounts {
		if opts.Filter != "" && account.Provider != opts.Filter {
			continue
		}
		active := ""
		if account.Active {
			active = " ← active"
		}
		email := ""
		if account.Email != "" {
			email = fmt.Sprintf(" [%s]", account.Email)
		}
		prefix := ""
		if showProvider {
			prefix = fmt.Sprintf("%-*s  ", providerColumnWidth, account.Provider)
		}
		fmt.Fprintf(w, "  %s%s (%s)%s%s\n", prefix, account.Name, account.ProjectID, email, active)
	}
}

const providerColumnWidth = 8

// Active prints `gctx active`.
func Active(w io.Writer, name string) {
	fmt.Fprintf(w, "Active account: %s\n", name)
}

// ActiveSlots prints `gctx active` for several slots: gcp, aws and azure
// first, then the others sorted.
func ActiveSlots(w io.Writer, slots map[string]string) {
	order := []string{"gcp", "aws", "azure"}
	var others []string
	for slot := range slots {
		if !slices.Contains(order, slot) {
			others = append(others, slot)
		}
	}
	sort.Strings(others)
	for _, slot := range append(order, others...) {
		if name, ok := slots[slot]; ok {
			fmt.Fprintf(w, "%s: %s\n", slot, name)
		}
	}
}

// Info prints `gctx info`.
func Info(w io.Writer, status manager.AccountStatus) {
	fmt.Fprintf(w, "\nAccount: %s\n", status.Name)
	fmt.Fprintln(w, strings.Repeat("=", 50))
	if status.Provider != "" && status.Provider != provider.GCP {
		fmt.Fprintf(w, "Provider:         %s\n", status.Provider)
		fmt.Fprintf(w, "%-18s%s\n", status.TargetLabel+":", status.ProjectID)
	} else {
		fmt.Fprintf(w, "Project ID:       %s\n", status.ProjectID)
	}
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
		if warning.Code == provider.WarnSetProjectFailed {
			continue // already shown while logging in
		}
		fmt.Fprintf(errw, "Warning: %s\n", warning.Message)
	}
	if result.ADCSaved || result.ADCPath != "" {
		fmt.Fprintf(w, "ADC credentials auto-saved for: %s\n", result.Account)
		fmt.Fprintf(w, "Saved to: %s\n", result.ADCPath)
	}
	fmt.Fprintln(w)
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
	// Global switches (gcp) replace silently, as before slots existed.
	if result.Replaced != "" && result.ApplyHint != "" {
		fmt.Fprintf(w, "Replaced %s in slot %s\n", result.Replaced, result.Slot)
	}
	if result.ApplyHint != "" {
		fmt.Fprintln(w, result.ApplyHint)
	}
}

// Env prints one POSIX `export KEY='value'` line per KEY=value entry.
func Env(w io.Writer, entries []string) {
	for _, entry := range entries {
		key, value, _ := strings.Cut(entry, "=")
		fmt.Fprintf(w, "export %s='%s'\n", key, strings.ReplaceAll(value, "'", `'\''`))
	}
}

// Save prints the outcome of `gctx save`.
func Save(w io.Writer, result manager.SaveResult) {
	fmt.Fprintf(w, "ADC credentials saved for: %s\n", result.Account)
	fmt.Fprintf(w, "Location: %s\n", result.ADCPath)
}

// Delete prints the outcome of `gctx delete`.
func Delete(w, errw io.Writer, result manager.DeleteResult) {
	fmt.Fprintf(w, "Deleted account: %s\n", result.Account)
	for _, warning := range result.Warnings {
		fmt.Fprintf(errw, "Warning: %s\n", warning.Message)
	}
}

// ShellBanner prints the text shown before a shell starts.
func ShellBanner(w io.Writer, name string) {
	fmt.Fprintf(w, "Starting shell for account '%s'...\n", name)
	fmt.Fprintln(w, "Type 'exit' to return to the previous context.")
}
