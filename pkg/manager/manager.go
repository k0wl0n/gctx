package manager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/gcloudconfig"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/aws"
	"github.com/k0wl0n/gctx/pkg/provider/gcp"
)

// credentialCheckTimeout bounds each provider's part of CheckCredentials.
const credentialCheckTimeout = 15 * time.Second

// Paths are the filesystem roots gctx reads and writes.
type Paths struct {
	GctxDir    string // ~/.config/gctx on every OS
	ADCDir     string // <GctxDir>/adc
	GcloudDir  string // $CLOUDSDK_CONFIG | %APPDATA%\gcloud | ~/.config/gcloud
	DefaultADC string // <GcloudDir>/application_default_credentials.json
}

// ConfigPath is the location of config.json.
func (p Paths) ConfigPath() string { return filepath.Join(p.GctxDir, "config.json") }

// ResolvePaths derives every root from the environment, OS and home directory.
func ResolvePaths(getenv func(string) string, goos, home string) Paths {
	gctxDir := filepath.Join(home, ".config", "gctx")
	gcloudDir := gcloudconfig.Dir(getenv, goos, home)
	return Paths{
		GctxDir:    gctxDir,
		ADCDir:     filepath.Join(gctxDir, "adc"),
		GcloudDir:  gcloudDir,
		DefaultADC: filepath.Join(gcloudDir, "application_default_credentials.json"),
	}
}

// Options injects everything a Manager takes from its environment.
type Options struct {
	Paths    Paths
	Gcloud   *gcloud.Client // nil: located on PATH, and re-located on every Overview
	Getenv   func(string) string
	LookPath func(string) (string, error)
	GOOS     string
	Now      func() time.Time

	// Providers replaces the default provider set built from Paths.
	Providers []provider.Provider
	// CredentialTimeout bounds each provider's credential check; zero means 15s.
	CredentialTimeout time.Duration
}

// DefaultOptions resolves options from the real environment.
func DefaultOptions() (Options, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Options{}, err
	}
	return Options{
		Paths:    ResolvePaths(os.Getenv, runtime.GOOS, home),
		Getenv:   os.Getenv,
		LookPath: exec.LookPath,
		GOOS:     runtime.GOOS,
		Now:      time.Now,
	}, nil
}

// Manager implements every account operation. It returns data and warnings and
// never prints; presenters render the results.
type Manager struct {
	opts      Options
	paths     Paths
	providers *provider.Registry

	// mu serialises every read and write of config.json.
	mu sync.Mutex

	// stateMu guards the session state below.
	stateMu sync.Mutex
	// credentialed holds each provider's last successful AuthReport.Credentialed:
	// lower-cased gcloud accounts for GCP, account names elsewhere.
	credentialed map[provider.ID]map[string]bool
	credChecked  map[provider.ID]bool
	reauth       map[string]bool // accounts that reported re-authentication required

	// config is the legacy cached document used by operations not yet converted.
	config *config.Config
}

// New creates a Manager for the real environment.
func New() (*Manager, error) {
	opts, err := DefaultOptions()
	if err != nil {
		return nil, err
	}
	m, err := NewWithOptions(opts)
	if err != nil {
		return nil, err
	}
	if m.config, err = config.LoadFrom(m.paths.ConfigPath()); err != nil {
		return nil, err
	}
	return m, nil
}

// NewWithOptions creates a Manager from explicit options. It reads nothing:
// an unreadable config.json surfaces from the operations that need it.
func NewWithOptions(opts Options) (*Manager, error) {
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Paths.GctxDir == "" {
		return nil, fmt.Errorf("manager: Options.Paths is required")
	}
	if opts.CredentialTimeout <= 0 {
		opts.CredentialTimeout = credentialCheckTimeout
	}
	providers := opts.Providers
	if providers == nil {
		providers = defaultProviders(opts)
	}
	m := &Manager{
		opts:         opts,
		paths:        opts.Paths,
		providers:    provider.NewRegistry(providers...),
		credentialed: map[provider.ID]map[string]bool{},
		credChecked:  map[provider.ID]bool{},
		reauth:       map[string]bool{},
	}
	return m, nil
}

// defaultProviders is the only place the manager names a concrete provider.
func defaultProviders(opts Options) []provider.Provider {
	return []provider.Provider{
		gcp.New(gcp.Options{
			GcloudDir:  opts.Paths.GcloudDir,
			ADCDir:     opts.Paths.ADCDir,
			DefaultADC: opts.Paths.DefaultADC,
			Client:     opts.Gcloud,
			LookPath:   opts.LookPath,
			Getenv:     opts.Getenv,
		}),
		aws.New(aws.Options{
			LookPath: opts.LookPath,
			Getenv:   opts.Getenv,
		}),
	}
}

// Paths returns the resolved filesystem roots.
func (m *Manager) Paths() Paths { return m.paths }

func (m *Manager) gcloudCLI() provider.CLIStatus {
	if p, ok := m.providers.Get(provider.GCP); ok {
		return p.CLI()
	}
	return provider.CLIStatus{}
}

// GcloudBin returns the gcloud executable, or "" when it is not installed.
func (m *Manager) GcloudBin() string { return m.gcloudCLI().Bin }

// GcloudAvailable reports whether a gcloud executable is installed.
func (m *Manager) GcloudAvailable() bool { return m.gcloudCLI().Found }

// providerFor returns the provider an account belongs to.
func (m *Manager) providerFor(account config.Account) (provider.Provider, error) {
	id := provider.ID(account.ProviderID())
	p, ok := m.providers.Get(id)
	if !ok {
		return nil, unknownProviderError{account: account.Name, id: id}
	}
	return p, nil
}

// providerIDs lists the registered provider ids in registry order.
func (m *Manager) providerIDs() []provider.ID {
	var ids []provider.ID
	for _, p := range m.providers.All() {
		ids = append(ids, p.ID())
	}
	return ids
}

func (m *Manager) loadConfig() (*config.Config, error) {
	return config.LoadFrom(m.paths.ConfigPath())
}
