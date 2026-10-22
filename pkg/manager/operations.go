package manager

import (
	"context"
	"fmt"
	"maps"
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
	account.Settings = mergeSettings(account.Settings, outcome.Settings)
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

// UpdateAccount renames an account and merges changed editable settings.
// Only the config entry is renamed: provider-side resources (gcloud
// configuration, ADC file, isolated directory, CLI profile) keep their old
// names because the account record stores their paths. The slot the account
// is active in follows the new name.
func (m *Manager) UpdateAccount(ctx context.Context, name string, req UpdateRequest) error {
	account, p, err := m.resolve(name)
	if err != nil {
		return err
	}
	renaming := req.NewName != "" && req.NewName != name
	if renaming {
		if err := ValidateAccountName(req.NewName); err != nil {
			return err
		}
	}
	editable := map[string]bool{}
	for _, field := range p.Form() {
		if field.Editable {
			editable[field.Key] = true
		}
	}
	for key := range req.Settings {
		if !editable[key] {
			return fmt.Errorf("setting %q cannot be changed after creation", key)
		}
	}
	settings := maps.Clone(account.Settings)
	for key, value := range req.Settings {
		if value == "" {
			delete(settings, key)
			continue
		}
		if settings == nil {
			settings = map[string]string{}
		}
		settings[key] = value
	}

	err = m.mutate(func(cfg *config.Config) error {
		stored, err := cfg.GetAccount(name)
		if err != nil {
			return notFoundError{name}
		}
		if renaming {
			if _, exists := cfg.Accounts[req.NewName]; exists {
				return existsError{req.NewName}
			}
			delete(cfg.Accounts, name)
			stored.Name = req.NewName
			cfg.Accounts[req.NewName] = stored
			for slot, active := range cfg.ActiveBySlot {
				if active == name {
					cfg.ActiveBySlot[slot] = req.NewName
				}
			}
			if cfg.ActiveAccount == name {
				cfg.ActiveAccount = req.NewName
			}
		}
		stored.Settings = settings
		return cfg.Save()
	})
	if err == nil && renaming {
		m.forgetCachedStatus(name)
	}
	return err
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
	m.forgetCachedStatus(name)
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
	result := SwitchResult{Account: name, ProjectID: account.ProjectID, Slot: p.Slot()}

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
		if previous := cfg.ActiveBySlot[result.Slot]; previous != name {
			result.Replaced = previous
		}
		if err := cfg.SetActiveSlot(result.Slot, name); err != nil {
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
	dependants, err := m.dependants(name)
	if err != nil {
		return DeleteResult{}, err
	}
	if len(dependants) > 0 {
		return DeleteResult{}, ErrAccountLinked{Account: name, Dependants: dependants}
	}
	result := DeleteResult{Account: name}

	removed, err := p.Delete(ctx, account, opt)
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
	if err == nil {
		m.forgetCachedStatus(name)
	}
	return result, err
}

// mergeSettings returns base overlaid with extra, or base itself when extra is empty.
func mergeSettings(base, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	merged := make(map[string]string, len(base)+len(extra))
	maps.Copy(merged, base)
	maps.Copy(merged, extra)
	return merged
}

// dependants returns the sorted names of accounts that run on name (D12).
func (m *Manager) dependants(name string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.loadConfig()
	if err != nil {
		return nil, err
	}
	var names []string
	for other, account := range cfg.Accounts {
		if other != name && account.Settings["via"] == name {
			names = append(names, other)
		}
	}
	slices.Sort(names)
	return names, nil
}

// DeletePlan describes what deleting name offers to remove and keep.
func (m *Manager) DeletePlan(name string) (DeletePlan, error) {
	account, p, err := m.resolve(name)
	if err != nil {
		return DeletePlan{}, err
	}
	plan := DeletePlan{}
	if describer, ok := p.(provider.DeleteDescriber); ok {
		plan.Items, plan.Kept = describer.DeleteItems(account), describer.Kept(account)
	}
	plan.Dependants, err = m.dependants(name)
	return plan, err
}
