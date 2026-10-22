// Package provider defines the contract every cloud backend implements.
package provider

import (
	"context"
	"io"
	"strings"

	"github.com/k0wl0n/gctx/pkg/config"
)

// ID identifies a cloud provider.
type ID string

const (
	GCP     ID = "gcp"
	AWS     ID = "aws"
	Azure   ID = "azure"
	Tencent ID = "tencent"
)

// Provider is one cloud backend. Local reads files only; CheckAuth may use the network.
type Provider interface {
	ID() ID
	Label() string
	TargetLabel() string
	CLI() CLIStatus
	ValidateTarget(target string) error
	Create(ctx context.Context, acct config.Account) (CreateOutcome, error)
	Login(ctx context.Context, acct config.Account, lio LoginIO) (LoginResult, error)
	Activate(ctx context.Context, acct config.Account) (ActivateResult, error)
	Local(acct config.Account) LocalStatus
	CheckAuth(ctx context.Context, accts []config.Account) AuthReport
	Env(acct config.Account) ([]string, error)
	Delete(ctx context.Context, acct config.Account, opt DeleteOptions) (DeleteResult, error)
	Details(acct config.Account) []Field
}

// CLIStatus reports whether the provider's CLI is on PATH.
type CLIStatus struct {
	Bin   string
	Found bool
}

// Field is one provider-specific detail row.
type Field struct {
	Label string
	Value string
}

// WarningCode identifies a non-fatal condition reported by an operation.
type WarningCode string

const (
	WarnADCMissing          WarningCode = "adc-missing"
	WarnReauthRequired      WarningCode = "reauth-required"
	WarnSetProjectFailed    WarningCode = "set-project-failed"
	WarnConfigReused        WarningCode = "config-reused"
	WarnConfigDeleteFailed  WarningCode = "config-delete-failed"
	WarnADCRemoveFailed     WarningCode = "adc-remove-failed"
	WarnADCPathOutsideStore WarningCode = "adc-path-outside-store"
	WarnADCNotRefreshed     WarningCode = "adc-not-refreshed"
	WarnGcloudNotFound      WarningCode = "gcloud-not-found"
	WarnCredentialCheck     WarningCode = "credential-check-failed"
)

// Warning is a non-fatal condition. Message holds the underlying detail and
// can span several lines; use Summary for a one-line form.
type Warning struct {
	Code    WarningCode
	Account string
	Message string
}

// Summary returns the last non-empty line of the message.
func (w Warning) Summary() string {
	lines := strings.Split(strings.TrimSpace(w.Message), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Health is an account's primary status.
type Health string

const (
	HealthError           Health = "ERROR"
	HealthConfigMissing   Health = "CONFIG MISSING"
	HealthAuthRequired    Health = "AUTH REQUIRED"
	HealthADCMissing      Health = "ADC MISSING"
	HealthProjectMismatch Health = "PROJECT MISMATCH"
	HealthCLIMissing      Health = "CLI MISSING"
	HealthReady           Health = "READY"
)

// Finding is one health finding. Message is set for ERROR findings.
type Finding struct {
	Health  Health
	Message string
}

// LocalStatus is the file-based view of one account.
type LocalStatus struct {
	Status   Health
	Findings []Finding
	Identity string
	Fields   []Field
}

// AuthReport is the outcome of a provider's credential check. Warnings is
// non-empty when the check could not complete.
type AuthReport struct {
	Credentialed map[string]bool
	Warnings     []Warning
}

// LoginEventKind names a progress point in the login flow.
type LoginEventKind string

const (
	LoginAuthStarted      LoginEventKind = "auth-started"
	LoginAuthDone         LoginEventKind = "auth-done"
	LoginADCStarted       LoginEventKind = "adc-started"
	LoginSetProjectFailed LoginEventKind = "set-project-failed"
	LoginVerifyingADC     LoginEventKind = "verifying-adc"
)

// LoginEvent is one ordered progress notification.
type LoginEvent struct {
	Kind    LoginEventKind
	Message string // detail for LoginSetProjectFailed
}

// LoginIO is the terminal handed to the interactive login steps. Progress is
// called in order as the flow advances.
type LoginIO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Progress       func(LoginEvent)
	// NoBrowser skips the automatic browser launch and prints a URL instead.
	NoBrowser bool
}

// LoginResult is returned even when login fails part-way, so callers can tell
// which steps completed.
type LoginResult struct {
	Account        string
	Email          string
	ADCPath        string
	GcloudLoggedIn bool // `auth login` succeeded; gcloud CLI credentials were updated
	ADCSaved       bool
	Completed      []string
	Warnings       []Warning
}

type CreateOutcome struct {
	ConfigName string
	Target     string
	TargetSet  bool
	Warnings   []Warning
}

type ActivateResult struct {
	Account   string
	ApplyHint string
	Warnings  []Warning
}

type DeleteOptions struct {
	Logout       bool
	RemoveDir    bool
	GcloudConfig bool
}

type DeleteResult struct {
	Account  string
	Removed  []string
	Warnings []Warning
}
