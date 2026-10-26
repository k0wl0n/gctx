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
	WarnProfileIncomplete   = provider.WarnProfileIncomplete
)

const (
	LoginAuthStarted      = provider.LoginAuthStarted
	LoginAuthDone         = provider.LoginAuthDone
	LoginADCStarted       = provider.LoginADCStarted
	LoginSetProjectFailed = provider.LoginSetProjectFailed
	LoginVerifyingADC     = provider.LoginVerifyingADC
	LoginStepPending      = provider.LoginStepPending
	LoginStepStarted      = provider.LoginStepStarted
	LoginStepDone         = provider.LoginStepDone
	LoginStepFailed       = provider.LoginStepFailed
	LoginDeviceCode       = provider.LoginDeviceCode
	LoginOutput           = provider.LoginOutput
)

const (
	HealthError           = provider.HealthError
	HealthConfigMissing   = provider.HealthConfigMissing
	HealthAuthRequired    = provider.HealthAuthRequired
	HealthADCMissing      = provider.HealthADCMissing
	HealthProjectMismatch = provider.HealthProjectMismatch
	HealthCLIMissing      = provider.HealthCLIMissing
	HealthReady           = provider.HealthReady
	HealthExpired         = provider.HealthExpired
	HealthUnverified      = provider.HealthUnverified
	HealthBudgetLow       = provider.HealthBudgetLow
)

// gcloud steps reported by login errors (gcloud.StepError.Step).
const (
	StepAuthLogin = "auth login"
	StepADCLogin  = "auth application-default login"
)

// CreateRequest describes an account to create. An empty Provider means GCP.
// Fields carries provider-specific settings stored on the account.
type CreateRequest struct {
	Provider provider.ID
	Name     string
	Target   string
	Fields   map[string]string
}

// UpdateRequest changes an account. An empty NewName keeps the name; Settings
// are merged into the stored settings, an empty value removing the key.
type UpdateRequest struct {
	NewName  string
	Settings map[string]string
}

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
	// Slot is the slot the switch changed; Replaced is that slot's previously
	// active account, empty when none or unchanged.
	Slot     string
	Replaced string
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

// DeleteOptions selects the optional removals; see provider.DeleteItem keys.
type DeleteOptions = provider.DeleteOptions

// DeletePlan is what the delete dialog shows: the optional removals the
// provider offers, what is kept, and the accounts that link to this one.
type DeletePlan struct {
	Items      []provider.DeleteItem
	Kept       string
	Dependants []string // sorted; deleting is refused while there are any
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
	Category    provider.Category // "" for an unknown provider
	TargetLabel string            // provider's name for ProjectID, e.g. "PROJECT"; "" for an unknown provider
	ConfigName  string
	ProjectID   string
	Email       string
	CreatedAt   time.Time
	Active      bool              // the active account of its provider's slot
	Settings    map[string]string // provider settings from config.json; never secrets
	Details     []provider.Field  // provider's Details() rows

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

	// Last credential check, from this session or the status cache; zero
	// values when there is none.
	Identity    string // never a token
	ExpiresAt   time.Time
	Checked     time.Time
	LastGoodAge time.Duration // UNVERIFIED only: age of the last good result
	Budget      *provider.Budget
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

	ActiveAccount      string            // gctx active account of the gcp slot ("" when none)
	ActiveBySlot       map[string]string // slot -> active account name
	ShellAccount       string            // GCTX_ACCOUNT, when running inside `gctx shell`
	GcloudActiveConfig string            // globally active gcloud configuration

	DefaultADC        ADCIndicator
	DefaultADCPath    string
	DefaultADCExists  bool
	DefaultADCModTime time.Time

	GcloudAvailable bool

	Providers []ProviderInfo // every registered provider, in registry order
}

// ProviderInfo summarises one registered provider for the overview.
type ProviderInfo struct {
	ID           provider.ID
	Label        string
	Category     provider.Category
	Badge        provider.Badge
	Slot         string
	EnvBased     bool // activation only prints an env line (provider.EnvBased)
	NetworkAuth  bool // the credential check calls a remote service (provider.NetworkAuth)
	TargetLabel  string
	Columns      []provider.Column
	CLI          provider.CLIStatus
	CheckTimeout time.Duration // bound on one credential check
	Count        int           // accounts using this provider

	Form         []provider.FormField
	Footer       func(values map[string]string) (writes, never string) // nil: none
	InstallHint  string                                                // how to install the CLI; "" when unknown
	CanLogin     bool
	StreamsLogin bool // Login reports steps and output as events (provider.StreamingLogin)
	NoRename     bool // accounts cannot be renamed (provider.RenameSupport)
}

// Active returns the status of the gcp slot's active account, if any.
func (o Overview) Active() (AccountStatus, bool) {
	if o.ActiveAccount == "" {
		return AccountStatus{}, false
	}
	return o.Account(o.ActiveAccount)
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
	// NetworkFailed lists network-checked providers (provider.NetworkAuth) that
	// had accounts checked, every one of which failed with a non-auth error
	// (timeout, abandoned check or CLI network error) and none succeeded.
	NetworkFailed []provider.ID
}
