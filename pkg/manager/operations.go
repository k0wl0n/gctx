package manager

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/provider"
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

// resolve loads one account record and the provider it belongs to.
func (m *Manager) resolve(name string) (config.Account, provider.Provider, error) {
	account, err := m.account(name)
	if err != nil {
		return config.Account{}, nil, err
	}
	p, err := m.providerFor(account)
	if err != nil {
		return config.Account{}, nil, err
	}
	return account, p, nil
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

// Create creates a GCP account; see CreateAccount.
func (m *Manager) Create(ctx context.Context, name, projectID string) (CreateResult, error) {
	return m.CreateAccount(ctx, CreateRequest{Provider: provider.GCP, Name: name, Target: projectID})
}

// CreateAccount validates the request, has the provider create its CLI-side
// state, and records the account. It stops before login.
func (m *Manager) CreateAccount(ctx context.Context, req CreateRequest) (CreateResult, error) {
	if err := ValidateAccountName(req.Name); err != nil {
		return CreateResult{}, err
	}
	id := req.Provider
	if id == "" {
		id = provider.GCP
	}
	p, ok := m.providers.Get(id)
	if !ok {
		return CreateResult{}, NewUnknownProviderError(id, m.providerIDs())
	}
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return CreateResult{}, err
	}
	if _, exists := cfg.Accounts[req.Name]; exists {
		return CreateResult{}, existsError{req.Name}
	}
	if err := p.ValidateTarget(req.Target); err != nil {
		return CreateResult{}, err
	}
	if !p.CLI().Found {
		if id == provider.GCP {
			return CreateResult{}, gcloud.ErrNotFound
		}
		return CreateResult{}, fmt.Errorf("%s CLI not found on PATH", p.Label())
	}

	account := config.Account{Name: req.Name, ProjectID: req.Target, Settings: req.Fields}
	if id != provider.GCP {
		account.Provider = string(id)
		if account.Settings == nil {
			account.Settings = map[string]string{} // providers record derived settings here
		}
	}
	var warnings []Warning
	if preparer, ok := p.(provider.CreatePreparer); ok {
		existing := make([]config.Account, 0, len(cfg.Accounts))
		for _, other := range cfg.Accounts {
			existing = append(existing, *other)
		}
		account, warnings = preparer.PrepareCreate(account, existing)
	}

	outcome, err := p.Create(ctx, account)
	result := CreateResult{
		Account:    req.Name,
		ConfigName: account.ConfigName,
		ProjectID:  req.Target,
		ProjectSet: outcome.TargetSet,
		Warnings:   append(warnings, outcome.Warnings...),
	}
	if outcome.ConfigName != "" {
		account.ConfigName, result.ConfigName = outcome.ConfigName, outcome.ConfigName
	}
	if err != nil {
		return result, err
	}

	account.CreatedAt = m.opts.Now()
	err = m.mutate(func(cfg *config.Config) error { return cfg.AddAccount(&account) })
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return result, existsError{req.Name}
	}
	return result, err
}

// Login runs the interactive gcloud login and ADC login for an account, then
// stores the resulting ADC. The result reports how far it got on failure.
func (m *Manager) Login(ctx context.Context, name string, lio LoginIO) (LoginResult, error) {
	result := LoginResult{Account: name}
	account, p, err := m.resolve(name)
	if err != nil {
		return result, err
	}
	result, err = p.Login(ctx, account, lio)
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
	account, p, err := m.resolve(name)
	if err != nil {
		return SaveResult{}, err
	}
	saver, ok := p.(provider.CredentialSaver)
	if !ok {
		return SaveResult{}, fmt.Errorf("%s accounts have no default credentials to save", p.Label())
	}
	adcPath, email, err := saver.SaveCredentials(ctx, account)
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

// Switch activates the account through its provider (for GCP: restores the
// ADC, activates the gcloud configuration, re-applies the project) and
// records it as active. Providers without a global switch return ApplyHint.
func (m *Manager) Switch(ctx context.Context, name string) (SwitchResult, error) {
	account, p, err := m.resolve(name)
	if err != nil {
		return SwitchResult{}, err
	}
	result := SwitchResult{Account: name, ProjectID: account.ProjectID}

	activated, err := p.Activate(ctx, account)
	result.Warnings, result.ApplyHint = activated.Warnings, activated.ApplyHint
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
	account, p, err := m.resolve(name)
	if err != nil {
		return DeleteResult{}, err
	}
	result := DeleteResult{Account: name}

	removed, err := p.Delete(ctx, account, provider.DeleteOptions{GcloudConfig: opt.GcloudConfig})
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
