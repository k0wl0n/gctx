package manager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/k0wl0n/gctx/pkg/adc"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/gcloudconfig"
)

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
	opts       Options
	paths      Paths
	store      adc.Store
	gcloud     *gcloud.Client
	autoGcloud bool

	// mu serialises every read and write of config.json.
	mu sync.Mutex

	// stateMu guards the session state below.
	stateMu      sync.Mutex
	credentialed map[string]bool // lower-cased gcloud accounts from the last check
	credChecked  bool
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
	m := &Manager{
		opts:         opts,
		paths:        opts.Paths,
		store:        adc.Store{Dir: opts.Paths.ADCDir, DefaultPath: opts.Paths.DefaultADC},
		gcloud:       opts.Gcloud,
		credentialed: map[string]bool{},
		reauth:       map[string]bool{},
	}
	if m.gcloud == nil {
		m.autoGcloud = true
		m.locateGcloud()
	}
	return m, nil
}

// locateGcloud looks gcloud up on PATH when the client was not injected.
func (m *Manager) locateGcloud() {
	if !m.autoGcloud {
		return
	}
	client := &gcloud.Client{}
	if bin, err := m.opts.LookPath("gcloud"); err == nil {
		client.Bin = bin
	}
	m.stateMu.Lock()
	m.gcloud = client
	m.stateMu.Unlock()
}

// client returns the current gcloud client.
func (m *Manager) client() *gcloud.Client {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.gcloud
}

// Paths returns the resolved filesystem roots.
func (m *Manager) Paths() Paths { return m.paths }

// GcloudBin returns the gcloud executable, or "" when it is not installed.
func (m *Manager) GcloudBin() string { return m.client().Bin }

// GcloudAvailable reports whether a gcloud executable is installed.
func (m *Manager) GcloudAvailable() bool { return m.client().Available() }

func (m *Manager) loadConfig() (*config.Config, error) {
	return config.LoadFrom(m.paths.ConfigPath())
}
