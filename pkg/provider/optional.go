package provider

import (
	"context"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
)

// The interfaces below are optional capabilities. Callers check for them with
// a type assertion; callers must not assume any is implemented.

// EnvBased marks a provider whose activation is only an environment: switching
// changes gctx's active account but no global state, so the user applies it
// with `env` (eval line). Providers without it switch globally.
type EnvBased interface {
	EnvBased() bool
}

// InspectedStatus is LocalStatus plus GCP's stored-ADC and gcloud
// configuration details.
type InspectedStatus struct {
	Local LocalStatus

	StoredADCPath      string // file actually holding the stored ADC; "" when none
	ADCExists          bool
	ADCModTime         time.Time
	RecordedADCModTime time.Time // mtime of acct.ADCPath; zero when it cannot be read

	ConfigExists  bool
	ConfigAccount string // [core] account of the account's gcloud configuration
	ConfigProject string // [core] project of the account's gcloud configuration
}

// StatusInspector reports file-based status with provider-specific details.
// Local.Identity is the key the provider uses in AuthReport.Results.
type StatusInspector interface {
	Inspect(acct config.Account) InspectedStatus
}

// CredentialSaver captures the provider's current default credentials into
// the account (`save`).
type CredentialSaver interface {
	SaveCredentials(ctx context.Context, acct config.Account) (credentialPath, email string, err error)
}

// AuthLister is CheckAuth that also returns every identity the CLI lists.
type AuthLister interface {
	ListAuth(ctx context.Context) ([]string, AuthReport)
}

// ConfigChecker reports whether a CLI configuration of that name exists.
type ConfigChecker interface {
	ConfigExists(configName string) bool
}

// CreatePreparer derives provider-specific account fields (such as the CLI
// configuration name) before Create, given every existing account, and may
// warn about conditions the user should know about.
type CreatePreparer interface {
	PrepareCreate(acct config.Account, existing []config.Account) (config.Account, []Warning)
}

// GlobalState is machine-wide CLI state outside gctx.
type GlobalState struct {
	ActiveConfig             string
	DefaultCredentialPath    string
	DefaultCredentialExists  bool
	DefaultCredentialModTime time.Time
	// Compared is false when there was nothing to compare against or a file
	// could not be read; Matches is meaningful only when Compared.
	Compared bool
	Matches  bool
}

// GlobalStateReader reads global state and compares the default credential
// with storedCredentialPath (may be ""). It reads files only.
type GlobalStateReader interface {
	GlobalState(storedCredentialPath string) GlobalState
}

// FormFooter states what creating an account writes and what it never touches,
// shown under the create form. values holds the form values plus FormNameKey.
type FormFooter interface {
	Footer(values map[string]string) (writes, never string)
}

// Installer tells the user how to install a missing CLI.
type Installer interface {
	InstallHint() string
}

// NetworkAuth marks a provider whose credential check calls a remote service.
// Its failures can mean the network is down; a provider without it (GCP's
// local `gcloud auth list`) never counts towards "offline".
type NetworkAuth interface {
	ChecksOverNetwork() bool
}

// LoginCapable reports whether the provider has an interactive login. A
// provider without it is assumed to have one.
type LoginCapable interface {
	HasLogin() bool
}

// StreamingLogin marks a provider whose Login reports its steps, device code
// and CLI output through LoginIO.Progress, so a UI can show them itself
// instead of handing over the terminal.
type StreamingLogin interface {
	StreamsLogin() bool
}

// SettingConfigOrigin marks an account whose CLI configuration existed before
// gctx and so is not gctx's to delete.
const (
	SettingConfigOrigin  = "config_origin"
	ConfigOriginImported = "imported" // adopted by the first-run import
	ConfigOriginExisting = "existing" // reused by create
)

// Delete item keys name the DeleteOptions field an item controls.
const (
	DeleteKeyLogout       = "logout"
	DeleteKeyRemoveDir    = "remove_dir"
	DeleteKeyGcloudConfig = "gcloud_config"
)

// DeleteItem is one removable thing offered by the delete dialog. Locked items
// cannot be toggled and stay at Default.
type DeleteItem struct {
	Key, Label, Detail string
	Default, Locked    bool
}

// DeleteDescriber lists what deleting an account can remove beyond the
// account record, and what it leaves alone. DeleteItems must be cheap: it
// runs when the dialog opens and may only read files.
type DeleteDescriber interface {
	DeleteItems(acct config.Account) []DeleteItem
	Kept(acct config.Account) string
}

// Set turns the option named by a DeleteItem key on or off.
func (o *DeleteOptions) Set(key string, on bool) {
	switch key {
	case DeleteKeyLogout:
		o.Logout = on
	case DeleteKeyRemoveDir:
		o.RemoveDir = on
	case DeleteKeyGcloudConfig:
		o.GcloudConfig = on
	}
}

// RenameSupport lets a provider whose resources follow the account name, and
// that cannot move them, refuse renames. A provider without it can rename.
type RenameSupport interface {
	CanRename() bool
}

// Renamer moves the provider resources named after an account when the
// account is renamed and returns the updated record. It refuses when a
// destination already exists.
type Renamer interface {
	Rename(ctx context.Context, acct config.Account, newName string) (config.Account, error)
}

// Claimer lists the resources an account owns, or would own once created, as
// human-readable strings ("gcloud configuration work-config"). No two
// accounts may claim one resource, and Delete leaves alone any claim that
// another account also holds (DeleteOptions.Keep).
type Claimer interface {
	Claims(acct config.Account) []string
}

// ConnectionResult is the outcome of ConnectionTester. Detail is one line for
// the user and never holds a key or token.
type ConnectionResult struct {
	Detail string
}

// ConnectionTester re-runs the provider's connectivity and budget check for
// one account that may not be saved yet (the create form's "Test again").
// A failure is returned as an error.
type ConnectionTester interface {
	TestConnection(ctx context.Context, acct config.Account) (ConnectionResult, error)
}
