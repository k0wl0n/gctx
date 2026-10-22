package manager

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/gcp"
)

// account loads one account record.
func (m *Manager) account(name string) (config.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.loadConfig()
	if err != nil {
		return config.Account{}, err
	}
	account, err := cfg.GetAccount(name)
	if err != nil {
		return config.Account{}, notFoundError{name}
	}
	return *account, nil
}

// mutate re-reads config.json under the lock, applies change, and leaves
// persisting to change itself or to the Save it requests by returning nil.
func (m *Manager) mutate(change func(*config.Config) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}
	return change(cfg)
}

// Create validates the name, creates and activates the gcloud configuration,
// tries to set the project, and records the account. It stops before login.
func (m *Manager) Create(ctx context.Context, name, projectID string) (CreateResult, error) {
	if err := ValidateAccountName(name); err != nil {
		return CreateResult{}, err
	}
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return CreateResult{}, err
	}
	if _, exists := cfg.Accounts[name]; exists {
		return CreateResult{}, existsError{name}
	}
	m.gcp.Locate()
	if !m.gcp.CLI().Found {
		return CreateResult{}, gcloud.ErrNotFound
	}

	configName := gcp.ConfigNameFor(name)
	result := CreateResult{Account: name, ConfigName: configName, ProjectID: projectID}
	if m.gcp.ConfigExists(configName) && !referencesConfig(cfg, configName) {
		result.Warnings = append(result.Warnings, Warning{
			Code: WarnConfigReused, Account: name,
			Message: fmt.Sprintf("gcloud configuration %s already exists and will be reused", configName),
		})
	}

	outcome, err := m.gcp.Create(ctx, config.Account{Name: name, ConfigName: configName, ProjectID: projectID})
	result.Warnings = append(result.Warnings, outcome.Warnings...)
	result.ProjectSet = outcome.TargetSet
	if err != nil {
		return result, err
	}

	err = m.mutate(func(cfg *config.Config) error {
		return cfg.AddAccount(&config.Account{
			Name:       name,
			ConfigName: configName,
			ProjectID:  projectID,
			CreatedAt:  m.opts.Now(),
		})
	})
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return result, existsError{name}
	}
	return result, err
}

func referencesConfig(cfg *config.Config, configName string) bool {
	for _, account := range cfg.Accounts {
		if account.ConfigName == configName {
			return true
		}
	}
	return false
}

// Login runs the interactive gcloud login and ADC login for an account, then
// stores the resulting ADC. The result reports how far it got on failure.
func (m *Manager) Login(ctx context.Context, name string, lio LoginIO) (LoginResult, error) {
	result := LoginResult{Account: name}
	account, err := m.account(name)
	if err != nil {
		return result, err
	}
	result, err = m.gcp.Login(ctx, account, lio)
	if err != nil {
		return result, err
	}

	err = m.mutate(func(cfg *config.Config) error {
		stored, err := cfg.GetAccount(name)
		if err != nil {
			return notFoundError{name}
		}
		stored.ADCPath, stored.Email = result.ADCPath, result.Email
		return cfg.Save()
	})
	if err != nil {
		return result, err
	}
	m.stateMu.Lock()
	delete(m.reauth, name)
	m.stateMu.Unlock()
	return result, nil
}

// SaveCredentials copies the default ADC into the account's stored location.
func (m *Manager) SaveCredentials(ctx context.Context, name string) (SaveResult, error) {
	account, err := m.account(name)
	if err != nil {
		return SaveResult{}, err
	}
	adcPath, email, err := m.gcp.SaveCredentials(ctx, account)
	if err != nil {
		return SaveResult{}, err
	}
	result := SaveResult{Account: name, ADCPath: adcPath, Email: email}
	err = m.mutate(func(cfg *config.Config) error {
		stored, err := cfg.GetAccount(name)
		if err != nil {
			return notFoundError{name}
		}
		stored.ADCPath, stored.Email = adcPath, result.Email
		return cfg.Save()
	})
	return result, err
}

// Switch restores the account's ADC, activates its gcloud configuration,
// re-applies its project, and records it as active.
func (m *Manager) Switch(ctx context.Context, name string) (SwitchResult, error) {
	account, err := m.account(name)
	if err != nil {
		return SwitchResult{}, err
	}
	result := SwitchResult{Account: name, ProjectID: account.ProjectID}

	activated, err := m.gcp.Activate(ctx, account)
	result.Warnings = activated.Warnings
	if slices.ContainsFunc(activated.Warnings, func(w Warning) bool { return w.Code == WarnReauthRequired }) {
		m.stateMu.Lock()
		m.reauth[name] = true
		m.stateMu.Unlock()
	}
	if err != nil {
		return result, err
	}

	err = m.mutate(func(cfg *config.Config) error {
		if err := cfg.SetActive(name); err != nil {
			return notFoundError{name}
		}
		return nil
	})
	return result, err
}

// Delete removes the account's stored ADC, optionally its gcloud
// configuration, and the account record. Failures after the first step are
// warnings so the record is still removed.
func (m *Manager) Delete(ctx context.Context, name string, opt DeleteOptions) (DeleteResult, error) {
	account, err := m.account(name)
	if err != nil {
		return DeleteResult{}, err
	}
	result := DeleteResult{Account: name}

	removed, err := m.gcp.Delete(ctx, account, provider.DeleteOptions{GcloudConfig: opt.GcloudConfig})
	result.Warnings = removed.Warnings
	result.ConfigDeleted = slices.Contains(removed.Removed, account.ConfigName)
	if err != nil {
		return result, err
	}

	err = m.mutate(func(cfg *config.Config) error {
		if err := cfg.DeleteAccount(name); err != nil {
			return notFoundError{name}
		}
		return nil
	})
	return result, err
}
