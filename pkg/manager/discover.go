package manager

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloudconfig"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// DiscoveredAccount is an existing CLI setup that can become an account. It
// holds names, targets and non-secret settings only.
type DiscoveredAccount struct {
	Provider provider.ID
	Name     string // already sanitized for use as an account name
	Target   string
	Settings map[string]string
	Note     string

	ConfigName string // GCP: the existing gcloud configuration, reused as is
	Email      string // GCP: [core] account of that configuration
}

// DiscoverySource summarises what one provider's discovery found.
type DiscoverySource struct {
	Provider provider.ID
	Count    int
	Noun     string // "gcloud configurations", "profiles in ~/.aws/config"
}

// DiscoveryResult is the outcome of Discover.
type DiscoveryResult struct {
	Sources []DiscoverySource
	Items   []DiscoveredAccount
	Skipped []SkippedSource // sources that could not be read; the others still count
}

// SkippedSource is a provider whose setup could not be read.
type SkippedSource struct {
	Provider provider.ID
	Reason   string
}

// SkippedImport is an item ImportAccounts did not import.
type SkippedImport struct {
	Name   string
	Reason string
}

// ImportResult lists the accounts created and the items skipped.
type ImportResult struct {
	Imported []string
	Skipped  []SkippedImport
}

// secretSettingParts mark a setting name that must never be stored.
var secretSettingParts = []string{"key", "token", "secret", "password", "credential"}

// Discover looks for CLI setups gctx can import. It reads configuration files
// only: no subprocess, no network, and never a token or credential file.
func (m *Manager) Discover() (DiscoveryResult, error) {
	var result DiscoveryResult
	for _, p := range m.providers.All() {
		var items []DiscoveredAccount
		var noun string
		var err error
		switch p.ID() {
		case provider.GCP:
			items, err = m.discoverGCP()
			noun = "gcloud configurations"
		case provider.AWS:
			var path string
			items, path, err = m.discoverAWS()
			noun = "profiles in " + m.displayPath(path)
		case provider.Azure:
			items, err = m.discoverAzure()
			noun = "subscriptions in " + m.displayPath(m.azureDefaultDir())
		default:
			continue
		}
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedSource{Provider: p.ID(), Reason: err.Error()})
			continue
		}
		result.Sources = append(result.Sources, DiscoverySource{Provider: p.ID(), Count: len(items), Noun: noun})
		result.Items = append(result.Items, items...)
	}
	return result, nil
}

func (m *Manager) displayPath(path string) string {
	home := m.opts.Home
	if home == "" || path == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return path
}

// SanitizeAccountName turns a CLI-side name into a valid account name:
// lower-case, every other rune replaced by "-". The result may still be
// invalid (for example when it starts with a digit); callers validate it.
func SanitizeAccountName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		}
		return '-'
	}, name)
}

func (m *Manager) discoverGCP() ([]DiscoveredAccount, error) {
	entries, err := os.ReadDir(filepath.Join(m.paths.GcloudDir, "configurations"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []DiscoveredAccount
	for _, entry := range entries {
		configName, ok := strings.CutPrefix(entry.Name(), "config_")
		if !ok || entry.IsDir() {
			continue
		}
		cfg, err := gcloudconfig.Read(m.paths.GcloudDir, configName)
		if err != nil || !cfg.Exists {
			continue
		}
		items = append(items, DiscoveredAccount{
			Provider: provider.GCP, Name: SanitizeAccountName(configName), Target: cfg.Project,
			ConfigName: configName, Email: cfg.Account,
		})
	}
	return items, nil
}

// awsProfileKeys are the only keys read from the AWS config file.
var awsProfileKeys = []string{"region", "sso_session", "sso_start_url", "sso_region", "sso_account_id", "sso_role_name"}

func (m *Manager) awsConfigPath() string {
	if path := m.opts.Getenv("AWS_CONFIG_FILE"); path != "" {
		return path
	}
	if m.opts.Home == "" {
		return ""
	}
	return filepath.Join(m.opts.Home, ".aws", "config")
}

func (m *Manager) discoverAWS() ([]DiscoveredAccount, string, error) {
	path := m.awsConfigPath()
	if path == "" {
		return nil, path, nil
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	defer file.Close()

	profiles := map[string]map[string]string{}
	sessions := map[string]map[string]string{}
	var order []string
	var current map[string]string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			current = nil
			section := strings.TrimSpace(line[1 : len(line)-1])
			switch {
			case section == "default":
				current = map[string]string{}
				profiles["default"] = current
				order = append(order, "default")
			case strings.HasPrefix(section, "profile "):
				name := strings.TrimSpace(strings.TrimPrefix(section, "profile "))
				current = map[string]string{}
				profiles[name] = current
				order = append(order, name)
			case strings.HasPrefix(section, "sso-session "):
				current = map[string]string{}
				sessions[strings.TrimSpace(strings.TrimPrefix(section, "sso-session "))] = current
			}
		case current != nil:
			key, value, found := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			if found && slices.Contains(awsProfileKeys, key) {
				current[key] = strings.TrimSpace(value)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, path, err
	}

	var items []DiscoveredAccount
	for _, name := range order {
		keys := profiles[name]
		settings := map[string]string{"auth": "profile"}
		if keys["region"] != "" {
			settings["region"] = keys["region"]
		}
		note := ""
		if session, ok := sessions[keys["sso_session"]]; ok {
			for _, key := range []string{"sso_start_url", "sso_region"} {
				if keys[key] == "" {
					keys[key] = session[key]
				}
			}
		}
		if keys["sso_start_url"] != "" || keys["sso_session"] != "" {
			note = "sso profile"
			for _, key := range []string{"sso_start_url", "sso_region", "sso_account_id", "sso_role_name"} {
				if keys[key] != "" {
					settings[key] = keys[key]
				}
			}
		}
		items = append(items, DiscoveredAccount{
			Provider: provider.AWS, Name: SanitizeAccountName(name), Target: name, Settings: settings, Note: note,
		})
	}
	return items, path, nil
}

func (m *Manager) azureDefaultDir() string {
	if m.opts.Home == "" {
		return ""
	}
	return filepath.Join(m.opts.Home, ".azure")
}

// azureProfile decodes only subscription ids, names and tenants: any other
// field of azureProfile.json is ignored.
type azureProfile struct {
	Subscriptions []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		TenantID string `json:"tenantId"`
	} `json:"subscriptions"`
}

func (m *Manager) discoverAzure() ([]DiscoveredAccount, error) {
	dir := m.azureDefaultDir()
	if dir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "azureProfile.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var profile azureProfile
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(data), "\ufeff")), &profile); err != nil {
		return nil, fmt.Errorf("reading %s: %w", filepath.Join(dir, "azureProfile.json"), err)
	}
	seen := map[string]bool{}
	var items []DiscoveredAccount
	for _, sub := range profile.Subscriptions {
		if sub.ID == "" || seen[sub.ID] {
			continue
		}
		seen[sub.ID] = true
		name := SanitizeAccountName(sub.Name)
		settings := map[string]string{"dir": filepath.Join(m.paths.GctxDir, "azure", name)}
		if sub.TenantID != "" {
			settings["tenant"] = sub.TenantID
		}
		items = append(items, DiscoveredAccount{Provider: provider.Azure, Name: name, Target: sub.ID, Settings: settings})
	}
	return items, nil
}

// ImportAccounts records the items as accounts in config.json. It never runs
// a CLI: the accounts show AUTH REQUIRED or ADC MISSING until logged in, and
// nothing is created on the CLI side. An item that cannot be imported is
// reported in the result and does not stop the others.
func (m *Manager) ImportAccounts(items []DiscoveredAccount) (ImportResult, error) {
	var result ImportResult
	err := m.mutate(func(cfg *config.Config) error {
		for _, item := range items {
			name := SanitizeAccountName(item.Name)
			if reason := m.importBlock(cfg, item, name); reason != "" {
				result.Skipped = append(result.Skipped, SkippedImport{Name: item.Name, Reason: reason})
				continue
			}
			account := &config.Account{
				Name: name, ProjectID: item.Target, ConfigName: item.ConfigName, Email: item.Email,
				Settings: safeSettings(item.Settings), CreatedAt: m.opts.Now(),
			}
			if item.Provider == provider.GCP {
				// The configuration is the user's own: mark it so delete keeps it by default.
				if account.Settings == nil {
					account.Settings = map[string]string{}
				}
				account.Settings[provider.SettingConfigOrigin] = provider.ConfigOriginImported
			}
			if item.Provider != provider.GCP {
				account.Provider = string(item.Provider)
			}
			cfg.Accounts[name] = account
			result.Imported = append(result.Imported, name)
		}
		if len(result.Imported) == 0 {
			return nil
		}
		return cfg.Save()
	})
	return result, err
}

// importBlock says why item cannot become an account called name, or "".
func (m *Manager) importBlock(cfg *config.Config, item DiscoveredAccount, name string) string {
	p, ok := m.providers.Get(item.Provider)
	if !ok {
		return fmt.Sprintf("provider %q is not available", item.Provider)
	}
	if err := ValidateAccountName(name); err != nil {
		return "invalid account name"
	}
	if _, exists := cfg.Accounts[name]; exists {
		return "an account with this name already exists"
	}
	if item.Provider == provider.GCP && item.Target == "" {
		return "no project set in the gcloud configuration"
	}
	if item.Provider == provider.GCP && item.ConfigName == "" {
		return "no gcloud configuration name"
	}
	if err := p.ValidateTarget(item.Target); err != nil {
		return "invalid target: " + err.Error()
	}
	candidate := config.Account{Name: name, ProjectID: item.Target, ConfigName: item.ConfigName, Settings: item.Settings}
	if err := m.claimOwner(p, candidate, cfg, ""); err != nil {
		return err.Error()
	}
	return ""
}

// safeSettings drops any setting whose name looks like it holds a secret.
func safeSettings(settings map[string]string) map[string]string {
	var out map[string]string
	for key, value := range settings {
		lower := strings.ToLower(key)
		if slices.ContainsFunc(secretSettingParts, func(part string) bool { return strings.Contains(lower, part) }) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[key] = value
	}
	return out
}

// cliVersionTimeout bounds each `<cli> --version`; a variable so tests can shorten it.
var cliVersionTimeout = 3 * time.Second

// CLIVersions runs `<cli> --version` for every provider whose CLI is on PATH,
// concurrently, and returns the first output line of each. A CLI that fails or
// does not answer in time yields an empty version. It is meant for help and
// diagnostics, not for the startup path.
func (m *Manager) CLIVersions(ctx context.Context) map[provider.ID]string {
	var mu sync.Mutex
	var wg sync.WaitGroup
	versions := map[provider.ID]string{}
	for _, p := range m.providers.All() {
		cli := p.CLI()
		if !cli.Found || cli.Bin == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			version := cliVersion(ctx, cli.Bin)
			mu.Lock()
			versions[p.ID()] = version
			mu.Unlock()
		}()
	}
	wg.Wait()
	return versions
}

func cliVersion(ctx context.Context, bin string) string {
	ctx, cancel := context.WithTimeout(ctx, cliVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
