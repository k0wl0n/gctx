package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Config struct {
	Accounts      map[string]*Account `json:"accounts"`
	ActiveAccount string              `json:"active_account,omitempty"`
	// ActiveBySlot maps a slot (gcp, aws, ...) to its active account.
	ActiveBySlot map[string]string `json:"active_by_slot,omitempty"`

	// path is the file this config was loaded from and is saved to.
	path string
}

// gcpSlot is the slot mirrored into the legacy active_account field.
const gcpSlot = "gcp"

// MarshalJSON keeps the gcp slot only in active_account, so an older binary
// that rewrites that field never leaves a stale copy in active_by_slot, and
// GCP-only files stay byte-identical to the previous schema.
func (c Config) MarshalJSON() ([]byte, error) {
	type plain Config
	out := plain(c)
	out.ActiveAccount = c.ActiveBySlot[gcpSlot]
	out.ActiveBySlot = nil
	for slot, name := range c.ActiveBySlot {
		if slot == gcpSlot || name == "" {
			continue
		}
		if out.ActiveBySlot == nil {
			out.ActiveBySlot = map[string]string{}
		}
		out.ActiveBySlot[slot] = name
	}
	return json.Marshal(out)
}

// AppName names the config directory under ~/.config.
const AppName = "gctx"

// defaultProvider applies to accounts written before multi-cloud support,
// which have no provider field.
const defaultProvider = "gcp"

type Account struct {
	Name       string `json:"name"`
	ConfigName string `json:"config_name"`
	// ProjectID holds the provider's target (GCP project, AWS profile, ...).
	// The JSON key stays "project_id" for every provider so older gctx
	// versions can still read the file.
	ProjectID string            `json:"project_id"`
	ADCPath   string            `json:"adc_path"`
	CreatedAt time.Time         `json:"created_at"`
	Email     string            `json:"email,omitempty"`
	Provider  string            `json:"provider,omitempty"`
	Settings  map[string]string `json:"settings,omitempty"`
}

// ProviderID returns the account's provider, defaulting to GCP for v1 configs.
func (a Account) ProviderID() string {
	if a.Provider == "" {
		return defaultProvider
	}
	return a.Provider
}

// Target returns the provider-specific target the account points at.
func (a Account) Target() string {
	return a.ProjectID
}

func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", AppName), nil
}

func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config from the default location.
func Load() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads the config at path. A missing file yields an empty config
// that will be saved to path.
func LoadFrom(path string) (*Config, error) {
	config := &Config{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		config.Accounts = make(map[string]*Account)
		return config, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}
	if config.Accounts == nil {
		config.Accounts = make(map[string]*Account)
	}
	if config.ActiveAccount != "" {
		if config.ActiveBySlot == nil {
			config.ActiveBySlot = map[string]string{}
		}
		if config.ActiveBySlot[gcpSlot] == "" {
			config.ActiveBySlot[gcpSlot] = config.ActiveAccount
		}
	}
	return config, nil
}

// Path returns the file the config is saved to.
func (c *Config) Path() string {
	return c.path
}

// Save writes the config atomically: readers see either the previous or the
// new document, never a partial one.
func (c *Config) Save() error {
	path := c.path
	if path == "" {
		var err error
		if path, err = GetConfigPath(); err != nil {
			return err
		}
		c.path = path
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".config-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (c *Config) AddAccount(account *Account) error {
	if _, exists := c.Accounts[account.Name]; exists {
		return fmt.Errorf("account '%s' already exists", account.Name)
	}
	c.Accounts[account.Name] = account
	return c.Save()
}

func (c *Config) GetAccount(name string) (*Account, error) {
	account, exists := c.Accounts[name]
	if !exists {
		return nil, fmt.Errorf("account '%s' not found", name)
	}
	return account, nil
}

func (c *Config) DeleteAccount(name string) error {
	if _, exists := c.Accounts[name]; !exists {
		return fmt.Errorf("account '%s' not found", name)
	}
	delete(c.Accounts, name)
	for slot, active := range c.ActiveBySlot {
		if active == name {
			delete(c.ActiveBySlot, slot)
		}
	}
	if c.ActiveAccount == name {
		c.ActiveAccount = ""
	}
	return c.Save()
}

// ListAccounts returns all accounts sorted by name.
func (c *Config) ListAccounts() []*Account {
	accounts := make([]*Account, 0, len(c.Accounts))
	for _, acc := range c.Accounts {
		accounts = append(accounts, acc)
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Name < accounts[j].Name })
	return accounts
}

// SetActive makes name the active account of the gcp slot.
func (c *Config) SetActive(name string) error {
	return c.SetActiveSlot(gcpSlot, name)
}

// SetActiveSlot makes name the active account of slot and saves.
func (c *Config) SetActiveSlot(slot, name string) error {
	if _, exists := c.Accounts[name]; !exists {
		return fmt.Errorf("account '%s' not found", name)
	}
	if c.ActiveBySlot == nil {
		c.ActiveBySlot = map[string]string{}
	}
	c.ActiveBySlot[slot] = name
	if slot == gcpSlot {
		c.ActiveAccount = name
	}
	return c.Save()
}
