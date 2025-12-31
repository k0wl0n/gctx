package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Accounts      map[string]*Account `json:"accounts"`
	ActiveAccount string              `json:"active_account,omitempty"`
}

type Account struct {
	Name       string    `json:"name"`
	ConfigName string    `json:"config_name"`
	ProjectID  string    `json:"project_id"`
	ADCPath    string    `json:"adc_path"`
	CreatedAt  time.Time `json:"created_at"`
	Email      string    `json:"email,omitempty"`
}

func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gctx"), nil
}

func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Config{
			Accounts: make(map[string]*Account),
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.Accounts == nil {
		config.Accounts = make(map[string]*Account)
	}

	return &config, nil
}

func (c *Config) Save() error {
	dir, err := GetConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
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
	if c.ActiveAccount == name {
		c.ActiveAccount = ""
	}
	return c.Save()
}

func (c *Config) ListAccounts() []*Account {
	accounts := make([]*Account, 0, len(c.Accounts))
	for _, acc := range c.Accounts {
		accounts = append(accounts, acc)
	}
	return accounts
}

func (c *Config) SetActive(name string) error {
	if _, exists := c.Accounts[name]; !exists {
		return fmt.Errorf("account '%s' not found", name)
	}
	c.ActiveAccount = name
	return c.Save()
}
