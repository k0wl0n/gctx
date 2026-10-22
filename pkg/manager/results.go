package manager

import (
	"time"

	"github.com/k0wl0n/gctx/pkg/provider"
)

// Shared result types live in pkg/provider; these aliases keep callers unchanged.
type (
	WarningCode    = provider.WarningCode
	Warning        = provider.Warning
	Health         = provider.Health
	Finding        = provider.Finding
	LoginIO        = provider.LoginIO
	LoginEvent     = provider.LoginEvent
	LoginEventKind = provider.LoginEventKind
	LoginResult    = provider.LoginResult
)

const (
	WarnADCMissing          = provider.WarnADCMissing
	WarnReauthRequired      = provider.WarnReauthRequired
	WarnSetProjectFailed    = provider.WarnSetProjectFailed
	WarnConfigReused        = provider.WarnConfigReused
	WarnConfigDeleteFailed  = provider.WarnConfigDeleteFailed
	WarnADCRemoveFailed     = provider.WarnADCRemoveFailed
	WarnADCPathOutsideStore = provider.WarnADCPathOutsideStore
	WarnADCNotRefreshed     = provider.WarnADCNotRefreshed
	WarnGcloudNotFound      = provider.WarnGcloudNotFound
	WarnCredentialCheck     = provider.WarnCredentialCheck
)

const (
	LoginAuthStarted      = provider.LoginAuthStarted
	LoginAuthDone         = provider.LoginAuthDone
	LoginADCStarted       = provider.LoginADCStarted
	LoginSetProjectFailed = provider.LoginSetProjectFailed
	LoginVerifyingADC     = provider.LoginVerifyingADC
)

const (
	HealthError           = provider.HealthError
	HealthConfigMissing   = provider.HealthConfigMissing
	HealthAuthRequired    = provider.HealthAuthRequired
	HealthADCMissing      = provider.HealthADCMissing
	HealthProjectMismatch = provider.HealthProjectMismatch
	HealthCLIMissing      = provider.HealthCLIMissing
	HealthReady           = provider.HealthReady
)

// gcloud steps reported by login errors (gcloud.StepError.Step).
const (
	StepAuthLogin = "auth login"
	StepADCLogin  = "auth application-default login"
)

type CreateResult struct {
	Account    string
	ConfigName string
	ProjectID  string
	ProjectSet bool
	Warnings   []Warning
}

type SwitchResult struct {
	Account   string
	ProjectID string
	// ApplyHint tells the user how to apply an environment-based account to
	// their shell; empty for providers with a global switch.
	ApplyHint string
	Warnings  []Warning
}

type SaveResult struct {
	Account string
	ADCPath string
	Email   string
}

type DeleteOptions struct {
	GcloudConfig bool
}

type DeleteResult struct {
	Account       string
	ConfigDeleted bool
	Warnings      []Warning
}

// AccountStatus is the local view of one account.
type AccountStatus struct {
	Name        string
	Provider    provider.ID
	TargetLabel string // provider's name for ProjectID, e.g. "PROJECT"; "" for an unknown provider
	ConfigName  string
	ProjectID   string
	Email       string
	CreatedAt   time.Time
	Active      bool // the gctx active account

	ADCPath       string // path recorded in config.json
	StoredADCPath string // file actually holding the stored ADC; "" when none
	ADCExists     bool
	ADCModTime    time.Time

	RecordedADCModTime time.Time // mtime of the file at ADCPath; zero when it cannot be read

	GcloudConfigExists bool
	GcloudAccount      string // [core] account of the account's gcloud configuration
	GcloudProject      string // [core] project of the account's gcloud configuration

	Status   Health
	Findings []Finding // every applicable finding, highest priority first
}

// HasFinding reports whether the account has the given finding.
func (s AccountStatus) HasFinding(h Health) bool {
	for _, f := range s.Findings {
		if f.Health == h {
			return true
		}
	}
	return false
}

// ADCIndicator describes how the default ADC file relates to the active account.
type ADCIndicator string

const (
	ADCActive  ADCIndicator = "active"  // byte-identical to the active account's stored ADC
	ADCDiffers ADCIndicator = "differs" // both exist but differ
	ADCNone    ADCIndicator = "none"    // no default ADC file
	ADCUnknown ADCIndicator = "unknown" // nothing to compare against
)

// Overview is the local snapshot shown by the CLI list and the TUI.
type Overview struct {
	ConfigPath string
	Accounts   []AccountStatus // sorted by name

	ActiveAccount      string // gctx active account ("" when none)
	ShellAccount       string // GCTX_ACCOUNT, when running inside `gctx shell`
	GcloudActiveConfig string // globally active gcloud configuration

	DefaultADC        ADCIndicator
	DefaultADCPath    string
	DefaultADCExists  bool
	DefaultADCModTime time.Time

	GcloudAvailable bool

	Providers []ProviderInfo // every registered provider, in registry order
}

// ProviderInfo summarises one registered provider for the overview.
type ProviderInfo struct {
	ID    provider.ID
	Label string
	CLI   provider.CLIStatus
	Count int // accounts using this provider
}

// Active returns the active account's status, if any.
func (o Overview) Active() (AccountStatus, bool) {
	for _, account := range o.Accounts {
		if account.Active {
			return account, true
		}
	}
	return AccountStatus{}, false
}

// Account returns the named account's status, if present.
func (o Overview) Account(name string) (AccountStatus, bool) {
	for _, account := range o.Accounts {
		if account.Name == name {
			return account, true
		}
	}
	return AccountStatus{}, false
}

// CredentialReport is the outcome of the asynchronous credential check.
// Warnings is non-empty when the check could not complete; statuses then
// remain based on local inputs only.
type CredentialReport struct {
	Accounts []string
	Warnings []Warning
}
