// Package testutil provides a hermetic environment for gctx tests: a temporary
// home directory, a gcloud configuration directory, and a fake gcloud binary
// placed first on PATH.
package testutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	binDir    string
	buildErr  error
)

// exeName appends ".exe" on Windows.
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// FakeBinDir builds the fake gcloud (and its fakeshell twin) once per test
// process and returns the directory containing them.
func FakeBinDir(t testing.TB) string {
	t.Helper()
	buildOnce.Do(func() {
		binDir, buildErr = os.MkdirTemp("", "gctx-fakebin-")
		if buildErr != nil {
			return
		}
		gcloudPath := filepath.Join(binDir, exeName("gcloud"))
		cmd := exec.Command("go", "build", "-o", gcloudPath, "github.com/k0wl0n/gctx/internal/testutil/fakegcloud")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("building fake gcloud: %v\n%s", err, out)
			return
		}
		data, err := os.ReadFile(gcloudPath)
		if err != nil {
			buildErr = err
			return
		}
		buildErr = os.WriteFile(filepath.Join(binDir, exeName("fakeshell")), data, 0755)
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binDir
}

// Env is an isolated gctx/gcloud environment rooted in a temp directory.
type Env struct {
	t         testing.TB
	Root      string
	Home      string
	GcloudDir string
	GctxDir   string
	BinDir    string
	LogPath   string
}

// NewEnv creates the environment and points HOME, USERPROFILE, APPDATA,
// CLOUDSDK_CONFIG and PATH at it for the duration of the test.
//
// CLOUDSDK_CONFIG is set to the same directory gctx derived from HOME/APPDATA
// before it honoured CLOUDSDK_CONFIG, so tests are valid for both behaviours.
func NewEnv(t testing.TB) *Env {
	t.Helper()
	root := t.TempDir()
	env := &Env{
		t:       t,
		Root:    root,
		Home:    filepath.Join(root, "home"),
		BinDir:  FakeBinDir(t),
		LogPath: filepath.Join(root, "gcloud-invocations.jsonl"),
	}
	env.GctxDir = filepath.Join(env.Home, ".config", "gctx")
	if runtime.GOOS == "windows" {
		appData := filepath.Join(root, "appdata")
		t.Setenv("APPDATA", appData)
		env.GcloudDir = filepath.Join(appData, "gcloud")
	} else {
		env.GcloudDir = filepath.Join(env.Home, ".config", "gcloud")
	}
	for _, dir := range []string{env.Home, env.GcloudDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("HOME", env.Home)
	t.Setenv("USERPROFILE", env.Home)
	t.Setenv("CLOUDSDK_CONFIG", env.GcloudDir)
	t.Setenv("FAKE_GCLOUD_LOG", env.LogPath)
	t.Setenv("PATH", env.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{
		"CLOUDSDK_ACTIVE_CONFIG_NAME", "GCTX_ACCOUNT", "GOOGLE_APPLICATION_CREDENTIALS",
		"FAKE_GCLOUD_FAIL", "FAKE_GCLOUD_SLEEP", "FAKE_GCLOUD_ADC_UNCHANGED", "FAKE_GCLOUD_ACCOUNT",
		"FAKE_SHELL_EXIT", "GCTX_NO_TUI", "GCTX_DEBUG", "NO_COLOR", "GCTX_ASCII",
	} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	return env
}

// GcloudBin is the path of the fake gcloud executable.
func (e *Env) GcloudBin() string { return filepath.Join(e.BinDir, exeName("gcloud")) }

// FakeShell is the path of the fake shell executable.
func (e *Env) FakeShell() string { return filepath.Join(e.BinDir, exeName("fakeshell")) }

// HideGcloud removes the fake gcloud from PATH (and any real one) for the test.
func (e *Env) HideGcloud() {
	empty := filepath.Join(e.Root, "empty-bin")
	os.MkdirAll(empty, 0755)
	e.t.Setenv("PATH", empty)
}

func (e *Env) ConfigPath() string     { return filepath.Join(e.GctxDir, "config.json") }
func (e *Env) DefaultADCPath() string { return filepath.Join(e.GcloudDir, "application_default_credentials.json") }
func (e *Env) StoredADCPath(name string) string {
	return filepath.Join(e.GctxDir, "adc", name+"_adc.json")
}
func (e *Env) GcloudConfigPath(configName string) string {
	return filepath.Join(e.GcloudDir, "configurations", "config_"+configName)
}

// Account mirrors the config.json account schema.
type Account struct {
	Name       string    `json:"name"`
	ConfigName string    `json:"config_name"`
	ProjectID  string    `json:"project_id"`
	ADCPath    string    `json:"adc_path"`
	CreatedAt  time.Time `json:"created_at"`
	Email      string    `json:"email,omitempty"`
}

// Config mirrors the config.json schema.
type Config struct {
	Accounts      map[string]*Account `json:"accounts"`
	ActiveAccount string              `json:"active_account,omitempty"`
}

// SeedOptions controls what SeedAccount creates besides the config entry.
type SeedOptions struct {
	NoADC          bool   // skip the stored ADC file
	NoGcloudConfig bool   // skip the gcloud configuration file
	NoCredential   bool   // do not mark the account as credentialed in gcloud
	GcloudProject  string // project recorded in the gcloud config (default: project)
	Email          string // default: <name>@example.com
}

// SeedAccount writes an account to config.json and creates the matching gcloud
// configuration, credential, and stored ADC file.
func (e *Env) SeedAccount(name, project string, opts SeedOptions) {
	e.t.Helper()
	email := opts.Email
	if email == "" {
		email = name + "@example.com"
	}
	cfg := e.ReadConfig()
	account := &Account{
		Name:       name,
		ConfigName: name + "-config",
		ProjectID:  project,
		CreatedAt:  time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		Email:      email,
	}
	if !opts.NoADC {
		account.ADCPath = e.StoredADCPath(name)
		e.WriteFile(account.ADCPath, ADCJSON(name), 0600)
	}
	cfg.Accounts[name] = account
	e.WriteConfig(cfg)

	if !opts.NoGcloudConfig {
		gcloudProject := opts.GcloudProject
		if gcloudProject == "" {
			gcloudProject = project
		}
		e.WriteFile(e.GcloudConfigPath(account.ConfigName),
			[]byte(fmt.Sprintf("[core]\naccount = %s\nproject = %s\n", email, gcloudProject)), 0644)
	}
	if !opts.NoCredential {
		e.addCredential(email)
	}
}

// ADCJSON returns a deterministic fake ADC document for an account. The refresh
// token embeds a marker that tests use to assert credentials are never shown.
func ADCJSON(name string) []byte {
	return []byte(fmt.Sprintf(`{"client_id":"fake-client-id","client_secret":"fake-client-secret","refresh_token":"SECRET-REFRESH-%s","type":"authorized_user"}`, name))
}

func (e *Env) addCredential(email string) {
	path := filepath.Join(e.GcloudDir, "fake_credentialed_accounts.json")
	var accounts []string
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &accounts)
	}
	for _, a := range accounts {
		if a == email {
			return
		}
	}
	accounts = append(accounts, email)
	data, _ := json.Marshal(accounts)
	e.WriteFile(path, data, 0600)
}

// SetActive sets active_account in config.json.
func (e *Env) SetActive(name string) {
	cfg := e.ReadConfig()
	cfg.ActiveAccount = name
	e.WriteConfig(cfg)
}

// SetGcloudActiveConfig writes gcloud's active_config file.
func (e *Env) SetGcloudActiveConfig(name string) {
	e.WriteFile(filepath.Join(e.GcloudDir, "active_config"), []byte(name), 0644)
}

// GcloudActiveConfig reads gcloud's active_config file.
func (e *Env) GcloudActiveConfig() string {
	data, err := os.ReadFile(filepath.Join(e.GcloudDir, "active_config"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// GcloudProperty reads a [core] property from a gcloud configuration file.
func (e *Env) GcloudProperty(configName, key string) string {
	data, err := os.ReadFile(e.GcloudConfigPath(configName))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ReadConfig parses config.json, returning an empty config when absent.
func (e *Env) ReadConfig() *Config {
	e.t.Helper()
	cfg := &Config{Accounts: map[string]*Account{}}
	data, err := os.ReadFile(e.ConfigPath())
	if os.IsNotExist(err) {
		return cfg
	}
	if err != nil {
		e.t.Fatal(err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		e.t.Fatalf("parsing config.json: %v", err)
	}
	if cfg.Accounts == nil {
		cfg.Accounts = map[string]*Account{}
	}
	return cfg
}

// WriteConfig writes config.json in the same format gctx uses.
func (e *Env) WriteConfig(cfg *Config) {
	e.t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		e.t.Fatal(err)
	}
	e.WriteFile(e.ConfigPath(), data, 0644)
}

// WriteFile writes a file, creating parent directories.
func (e *Env) WriteFile(path string, data []byte, perm os.FileMode) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		e.t.Fatal(err)
	}
}

// Invocation is one recorded fake gcloud call.
type Invocation struct {
	Args           []string `json:"args"`
	Verb           string   `json:"verb"`
	ActiveConfig   string   `json:"active_config_env"`
	DisablePrompts string   `json:"disable_prompts"`
	ADCEnv         string   `json:"google_application_credentials"`
	StdinDevNull   bool     `json:"stdin_devnull"`
}

// Line returns the invocation's arguments joined by spaces.
func (i Invocation) Line() string { return strings.Join(i.Args, " ") }

// Invocations returns every fake gcloud call recorded so far.
func (e *Env) Invocations() []Invocation {
	e.t.Helper()
	f, err := os.Open(e.LogPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		e.t.Fatal(err)
	}
	defer f.Close()
	var calls []Invocation
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var call Invocation
		if err := json.Unmarshal(scanner.Bytes(), &call); err != nil {
			e.t.Fatal(err)
		}
		calls = append(calls, call)
	}
	return calls
}

// InvocationLines returns the argument lines of all recorded calls.
func (e *Env) InvocationLines() []string {
	var lines []string
	for _, call := range e.Invocations() {
		lines = append(lines, call.Line())
	}
	return lines
}

// ResetInvocations clears the invocation log.
func (e *Env) ResetInvocations() { os.Remove(e.LogPath) }
