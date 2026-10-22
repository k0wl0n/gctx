package provider

import (
	"context"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
)

// The interfaces below are optional capabilities. Callers check for them with
// a type assertion; today only GCP implements them.

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
