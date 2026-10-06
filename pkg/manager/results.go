package manager

import (
	"strings"
	"time"
)

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

// LoginResult is returned even when login fails part-way, so callers can tell
// which steps completed.
type LoginResult struct {
	Account        string
	ADCPath        string
	Email          string
	GcloudLoggedIn bool // `auth login` succeeded; gcloud CLI credentials were updated
	ADCSaved       bool
	Warnings       []Warning
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

// Health is an account's primary status.
type Health string

const (
	HealthError           Health = "ERROR"
	HealthConfigMissing   Health = "CONFIG MISSING"
	HealthAuthRequired    Health = "AUTH REQUIRED"
	HealthADCMissing      Health = "ADC MISSING"
	HealthProjectMismatch Health = "PROJECT MISMATCH"
	HealthReady           Health = "READY"
)

// Finding is one health finding. Message is set for ERROR findings.
type Finding struct {
	Health  Health
	Message string
}

// AccountStatus is the local view of one account.
type AccountStatus struct {
	Name       string
	ConfigName string
	ProjectID  string
	Email      string
	CreatedAt  time.Time
	Active     bool // the gctx active account

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
