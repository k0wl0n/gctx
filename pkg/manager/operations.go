package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/k0wl0n/gctx/pkg/adc"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/gcloudconfig"
)

// LoginIO is the terminal handed to the interactive login steps. Progress is
// called in order as the flow advances.
type LoginIO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Progress       func(LoginEvent)
}

func (l LoginIO) progress(kind LoginEventKind, message string) {
	if l.Progress != nil {
		l.Progress(LoginEvent{Kind: kind, Message: message})
	}
}

func accountEnv(configName string) []string {
	return []string{"CLOUDSDK_ACTIVE_CONFIG_NAME=" + configName}
}

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

func setProjectWarning(account string, err error) Warning {
	return Warning{Code: WarnSetProjectFailed, Account: account, Message: detailed(err)}
}

// detailed renders an error with gcloud's output appended, as the legacy
// "%w\n%s" messages did.
func detailed(err error) string {
	var stepErr *gcloud.StepError
	if errors.As(err, &stepErr) && stepErr.Output != "" {
		return fmt.Sprintf("%v\n%s", err, stepErr.Output)
	}
	return err.Error()
}

func (m *Manager) requireGcloud() error {
	m.locateGcloud()
	if !m.client().Available() {
		return gcloud.ErrNotFound
	}
	return nil
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
	if err := m.requireGcloud(); err != nil {
		return CreateResult{}, err
	}

	configName := name + "-config"
	result := CreateResult{Account: name, ConfigName: configName, ProjectID: projectID}
	if existing, _ := gcloudconfig.Read(m.paths.GcloudDir, configName); existing.Exists && !referencesConfig(cfg, configName) {
		result.Warnings = append(result.Warnings, Warning{
			Code: WarnConfigReused, Account: name,
			Message: fmt.Sprintf("gcloud configuration %s already exists and will be reused", configName),
		})
	}

	client := m.client()
	output, err := client.Output(ctx, gcloud.DefaultTimeout, nil, "config", "configurations", "create", configName)
	if err != nil && !strings.Contains(output, "already exists") {
		return result, fmt.Errorf("failed to create config: %w\n%s", err, output)
	}
	if _, err := client.Output(ctx, gcloud.DefaultTimeout, nil, "config", "configurations", "activate", configName); err != nil {
		return result, err
	}
	// A failure here is expected when the account is not authenticated yet.
	if _, err := client.Output(ctx, gcloud.SetProjectTimeout, accountEnv(configName), "config", "set", "project", projectID); err != nil {
		result.Warnings = append(result.Warnings, setProjectWarning(name, err))
	} else {
		result.ProjectSet = true
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
	if err := m.requireGcloud(); err != nil {
		return result, err
	}
	client := m.client()
	env := accountEnv(account.ConfigName)
	stdio := gcloud.Stdio{Stdin: lio.Stdin, Stdout: lio.Stdout, Stderr: lio.Stderr}

	if _, err := client.Output(ctx, gcloud.DefaultTimeout, nil, "config", "configurations", "activate", account.ConfigName); err != nil {
		return result, fmt.Errorf("failed to activate gcloud config: %w", err)
	}

	lio.progress(LoginAuthStarted, "")
	if err := client.Interactive(ctx, stdio, env, "auth", "login"); err != nil {
		return result, fmt.Errorf("auth login failed: %w", err)
	}
	result.GcloudLoggedIn = true
	lio.progress(LoginAuthDone, "")

	lio.progress(LoginADCStarted, "")
	before := adc.Snapshot(m.paths.DefaultADC)
	if err := client.Interactive(ctx, stdio, env, "auth", "application-default", "login"); err != nil {
		return result, fmt.Errorf("ADC auth failed: %w", err)
	}
	refreshed := before.Changed(adc.Snapshot(m.paths.DefaultADC))

	if account.ProjectID != "" {
		if _, err := client.Output(ctx, gcloud.SetProjectTimeout, env, "config", "set", "project", account.ProjectID); err != nil {
			warning := setProjectWarning(name, err)
			result.Warnings = append(result.Warnings, warning)
			lio.progress(LoginSetProjectFailed, warning.Message)
		}
	}

	lio.progress(LoginVerifyingADC, "")
	adcPath, err := m.store.Save(name)
	if err != nil {
		return result, err
	}
	if !refreshed {
		result.Warnings = append(result.Warnings, Warning{
			Code: WarnADCNotRefreshed, Account: name,
			Message: "gcloud did not rewrite the default ADC file; the existing file was saved",
		})
	}
	result.ADCSaved, result.ADCPath = true, adcPath
	result.Email = m.accountEmail(ctx, account.ConfigName)

	err = m.mutate(func(cfg *config.Config) error {
		stored, err := cfg.GetAccount(name)
		if err != nil {
			return notFoundError{name}
		}
		stored.ADCPath, stored.Email = adcPath, result.Email
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

// accountEmail reads core/account from the account's own configuration.
func (m *Manager) accountEmail(ctx context.Context, configName string) string {
	email, err := m.client().Query(ctx, gcloud.DefaultTimeout, accountEnv(configName), "config", "get-value", "account")
	if err != nil {
		return ""
	}
	return email
}

// SaveCredentials copies the default ADC into the account's stored location.
func (m *Manager) SaveCredentials(ctx context.Context, name string) (SaveResult, error) {
	account, err := m.account(name)
	if err != nil {
		return SaveResult{}, err
	}
	adcPath, err := m.store.Save(name)
	if err != nil {
		return SaveResult{}, err
	}
	m.locateGcloud()
	result := SaveResult{Account: name, ADCPath: adcPath}
	if m.client().Available() {
		result.Email = m.accountEmail(ctx, account.ConfigName)
	}
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

	if _, statErr := os.Stat(m.store.StoragePath(name)); statErr != nil {
		result.Warnings = append(result.Warnings, Warning{
			Code: WarnADCMissing, Account: name,
			Message: fmt.Sprintf("no saved ADC for account: %s", name),
		})
	} else if err := m.store.Restore(name); err != nil {
		return result, err
	}

	if err := m.requireGcloud(); err != nil {
		return result, err
	}
	client := m.client()
	if _, err := client.Output(ctx, gcloud.DefaultTimeout, nil, "config", "configurations", "activate", account.ConfigName); err != nil {
		return result, err
	}

	if _, err := client.Output(ctx, gcloud.SetProjectTimeout, accountEnv(account.ConfigName), "config", "set", "project", account.ProjectID); err != nil {
		warning := setProjectWarning(name, err)
		if strings.Contains(warning.Message, "Reauthentication required") {
			warning.Code = WarnReauthRequired
			m.stateMu.Lock()
			m.reauth[name] = true
			m.stateMu.Unlock()
		}
		result.Warnings = append(result.Warnings, warning)
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

	outside, err := m.store.Remove(name, account.ADCPath)
	if outside != "" {
		result.Warnings = append(result.Warnings, Warning{
			Code: WarnADCPathOutsideStore, Account: name,
			Message: fmt.Sprintf("recorded ADC path %s is outside the gctx ADC directory and was not deleted", outside),
		})
	}
	if err != nil {
		result.Warnings = append(result.Warnings, Warning{Code: WarnADCRemoveFailed, Account: name, Message: err.Error()})
	}

	if opt.GcloudConfig {
		m.locateGcloud()
		_, err := m.client().Output(ctx, gcloud.DefaultTimeout, nil, "config", "configurations", "delete", account.ConfigName, "--quiet")
		if err != nil {
			message := err.Error()
			var stepErr *gcloud.StepError
			if errors.As(err, &stepErr) && stepErr.LastLine() != "" {
				message = stepErr.LastLine()
			}
			result.Warnings = append(result.Warnings, Warning{Code: WarnConfigDeleteFailed, Account: name, Message: message})
		} else {
			result.ConfigDeleted = true
		}
	}

	err = m.mutate(func(cfg *config.Config) error {
		if err := cfg.DeleteAccount(name); err != nil {
			return notFoundError{name}
		}
		return nil
	})
	return result, err
}
