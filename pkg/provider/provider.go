// Package provider defines the contract every cloud backend implements.
package provider

import (
	"context"
	"io"
	"strings"
	"time"

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

	Category() Category
	Badge() Badge
	// Slot names the env vars or global state the provider owns; one account
	// per slot is active. Usually string(ID()).
	Slot() string
	// Columns are the provider's own columns for a single-provider view.
	Columns() []Column
	// Form lists the create-form fields after kind, provider and name.
	Form() []FormField
}

// Category groups providers for display; behavior never branches on it.
type Category string

const (
	CategoryCloud     Category = "cloud"
	CategoryAITool    Category = "ai-tool"
	CategoryAIGateway Category = "ai-gateway"
)

// BadgeStyle is how a badge is drawn.
type BadgeStyle string

const (
	BadgeFilled  BadgeStyle = "filled"
	BadgeOutline BadgeStyle = "outline"
	BadgeDashed  BadgeStyle = "dashed"
)

// Badge is the provider's short label; Color is a hex colour reference.
type Badge struct {
	Text  string
	Style BadgeStyle
	Color string
}

// AccountView is what a Column reads from one account.
type AccountView struct {
	Name     string
	Target   string
	Settings map[string]string
	Identity string
}

// Column is one provider-specific table column.
type Column struct {
	Key    string
	Header string
	Width  int
	Value  func(AccountView) string
}

// FormTargetKey is the FormField key of the account's target (project,
// profile, subscription).
const FormTargetKey = "target"

// FormNameKey is the key under which the account name is passed to Visible
// and FormFooter alongside the field values.
const FormNameKey = "name"

// FormField is one create-form input. Validate and Visible may be nil.
type FormField struct {
	Key      string
	Label    string
	Default  string
	Help     string
	Options  []string
	Validate func(string) error
	// Visible hides the field while it returns false for the current values
	// (nil: always shown). Hidden fields are neither validated nor submitted.
	Visible func(values map[string]string) bool
	// Editable fields may change after creation; the rest are identity and
	// read-only in the edit form.
	Editable bool
	// Secret fields accept only a key reference (op://, env:, ...), never a
	// raw secret value.
	Secret bool
}

// CLIStatus reports whether the provider's CLI is on PATH.
type CLIStatus struct {
	Name  string // executable looked up on PATH, e.g. "gcloud"; names it in messages when missing
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
	WarnProfileIncomplete   WarningCode = "profile-incomplete"
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
	HealthExpired         Health = "EXPIRED"
	HealthUnverified      Health = "UNVERIFIED"
	HealthBudgetLow       Health = "BUDGET LOW"
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
	// Results is keyed by account name, or by lower-cased gcloud account for GCP.
	Results  map[string]AuthResult
	Warnings []Warning
}

// AuthResult is one account's credential check. It never holds a token or key.
type AuthResult struct {
	Credentialed bool
	Identity     string    // email, ARN or org
	ExpiresAt    time.Time // zero: unknown
	Checked      time.Time // zero: stamped by the caller
	Budget       *Budget   // gateways only
	Err          error     // timeout or network error: no verdict (UNVERIFIED)
}

// Budget is a gateway's spend. Limit 0 means unlimited.
type Budget struct {
	Spent    float64 `json:"spent"`
	Limit    float64 `json:"limit"`
	Currency string  `json:"currency,omitempty"`
}

// LoginEventKind names a progress point in the login flow.
type LoginEventKind string

const (
	LoginAuthStarted      LoginEventKind = "auth-started"
	LoginAuthDone         LoginEventKind = "auth-done"
	LoginADCStarted       LoginEventKind = "adc-started"
	LoginSetProjectFailed LoginEventKind = "set-project-failed"
	LoginVerifyingADC     LoginEventKind = "verifying-adc"

	// Streaming providers (StreamingLogin) also report their steps, the
	// device code and every output line of the CLI.
	LoginStepPending LoginEventKind = "step-pending"
	LoginStepStarted LoginEventKind = "step-started"
	LoginStepDone    LoginEventKind = "step-done"
	LoginStepFailed  LoginEventKind = "step-failed"
	LoginDeviceCode  LoginEventKind = "device-code"
	LoginOutput      LoginEventKind = "output"
)

// LoginEvent is one ordered progress notification.
type LoginEvent struct {
	Kind    LoginEventKind
	Message string // detail for LoginSetProjectFailed and the step events
	Step    string // step label of the step events
	Code    string // device code of LoginDeviceCode
	URL     string // verification page of LoginDeviceCode
	Output  string // one CLI output line of LoginOutput
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
	// Settings are provider-owned account settings the manager stores.
	Settings map[string]string
	Warnings []Warning
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

// Stdio is the terminal handed to an interactive child process.
type Stdio struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Runner runs a provider CLI. Implementations own per-call timeouts.
type Runner interface {
	// Query runs a non-interactive command and returns its trimmed stdout.
	Query(ctx context.Context, env []string, args ...string) (string, error)
	// Interactive runs a command attached to the given terminal.
	Interactive(ctx context.Context, stdio Stdio, env []string, args ...string) error
	// WithEnv returns a Runner whose every child process also gets extra.
	WithEnv(extra []string) Runner
}

// Output streams of StreamingRunner.Stream.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

// StreamingRunner is an optional Runner capability: it runs a command with
// piped output and reports each line as it arrives. Callers type-assert for it.
type StreamingRunner interface {
	Stream(ctx context.Context, env []string, onLine func(stream, line string), args ...string) error
}

// CommandError describes a failed CLI invocation. Its message is the
// underlying error's.
type CommandError struct {
	Step   string // command arguments, e.g. "sts get-caller-identity"
	Err    error
	Output string // combined stdout and stderr of non-interactive runs
}

func (e *CommandError) Error() string { return e.Err.Error() }
func (e *CommandError) Unwrap() error { return e.Err }

// LastLine returns the last non-empty output line, for compact UI messages.
func (e *CommandError) LastLine() string {
	lines := strings.Split(strings.TrimSpace(e.Output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
