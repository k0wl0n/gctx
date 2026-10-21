package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/k0wl0n/gctx/pkg/adc"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/gcloudconfig"
)

const credentialCheckTimeout = 15 * time.Second

// ActiveAccount returns GCTX_ACCOUNT inside `gctx shell`, then the recorded
// active account, then "unknown".
func (m *Manager) ActiveAccount() string {
	if name := m.opts.Getenv("GCTX_ACCOUNT"); name != "" {
		return name
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg, err := m.loadConfig(); err == nil && cfg.ActiveAccount != "" {
		return cfg.ActiveAccount
	}
	return "unknown"
}

// Overview computes the local status of every account. It reads files only:
// no subprocess, no network, and no writes.
func (m *Manager) Overview() (Overview, error) {
	m.locateGcloud()
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return Overview{}, err
	}

	overview := Overview{
		ConfigPath:         m.paths.ConfigPath(),
		ActiveAccount:      cfg.ActiveAccount,
		ShellAccount:       m.opts.Getenv("GCTX_ACCOUNT"),
		GcloudActiveConfig: gcloudconfig.ActiveConfig(m.paths.GcloudDir),
		DefaultADCPath:     m.paths.DefaultADC,
		GcloudAvailable:    m.GcloudAvailable(),
	}
	for _, account := range cfg.ListAccounts() {
		overview.Accounts = append(overview.Accounts, m.accountStatus(cfg, account))
	}
	m.fillDefaultADC(&overview)
	return overview, nil
}

// Info returns the status of one account.
func (m *Manager) Info(name string) (AccountStatus, error) {
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return AccountStatus{}, err
	}
	account, err := cfg.GetAccount(name)
	if err != nil {
		return AccountStatus{}, notFoundError{name}
	}
	return m.accountStatus(cfg, account), nil
}

func (m *Manager) accountStatus(cfg *config.Config, account *config.Account) AccountStatus {
	status := AccountStatus{
		Name:       account.Name,
		ConfigName: account.ConfigName,
		ProjectID:  account.ProjectID,
		Email:      account.Email,
		CreatedAt:  account.CreatedAt,
		Active:     account.Name == cfg.ActiveAccount,
		ADCPath:    account.ADCPath,
	}
	if account.ADCPath != "" {
		if info, err := os.Stat(account.ADCPath); err == nil {
			status.RecordedADCModTime = info.ModTime()
		}
	}

	var errorFindings []Finding
	for _, candidate := range []string{m.store.StoragePath(account.Name), account.ADCPath} {
		if candidate == "" || (candidate == account.ADCPath && !m.store.Contains(candidate)) {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil {
			if !os.IsNotExist(err) {
				errorFindings = append(errorFindings, Finding{HealthError, "cannot read saved ADC file: " + err.Error()})
			}
			continue
		}
		if info.IsDir() {
			continue
		}
		status.StoredADCPath, status.ADCExists, status.ADCModTime = candidate, true, info.ModTime()
		if err := adc.Validate(candidate); err != nil {
			errorFindings = append(errorFindings, Finding{HealthError, "saved ADC file is invalid: " + err.Error()})
		}
		break
	}

	gcfg, err := gcloudconfig.Read(m.paths.GcloudDir, account.ConfigName)
	if err != nil {
		errorFindings = append(errorFindings, Finding{HealthError, err.Error()})
	}
	status.GcloudConfigExists = gcfg.Exists
	status.GcloudAccount, status.GcloudProject = gcfg.Account, gcfg.Project

	status.Findings = append(status.Findings, errorFindings...)
	if err == nil && !gcfg.Exists {
		status.Findings = append(status.Findings, Finding{Health: HealthConfigMissing})
	}
	if gcfg.Exists && m.authRequired(account.Name, gcfg.Account) {
		status.Findings = append(status.Findings, Finding{Health: HealthAuthRequired})
	}
	if !status.ADCExists && len(errorFindings) == 0 {
		status.Findings = append(status.Findings, Finding{Health: HealthADCMissing})
	}
	if gcfg.Project != "" && gcfg.Project != account.ProjectID {
		status.Findings = append(status.Findings, Finding{Health: HealthProjectMismatch})
	}
	status.Status = HealthReady
	if len(status.Findings) > 0 {
		status.Status = status.Findings[0].Health
	}
	return status
}

func (m *Manager) authRequired(name, gcloudAccount string) bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if gcloudAccount == "" || m.reauth[name] {
		return true
	}
	return m.credChecked && !m.credentialed[strings.ToLower(gcloudAccount)]
}

// fillDefaultADC compares the default ADC with the active account's stored ADC
// in memory; no content or hash is kept.
func (m *Manager) fillDefaultADC(overview *Overview) {
	info, err := os.Stat(m.paths.DefaultADC)
	if err != nil || info.IsDir() {
		overview.DefaultADC = ADCNone
		return
	}
	overview.DefaultADCExists, overview.DefaultADCModTime = true, info.ModTime()
	overview.DefaultADC = ADCUnknown
	active, ok := overview.Active()
	if !ok || active.StoredADCPath == "" {
		return
	}
	equal, err := adc.FilesEqual(m.paths.DefaultADC, active.StoredADCPath)
	switch {
	case err != nil:
	case equal:
		overview.DefaultADC = ADCActive
	default:
		overview.DefaultADC = ADCDiffers
	}
}

// CheckCredentials lists the accounts gcloud holds credentials for, without
// network access and without printing or refreshing any token. A failure is
// reported as a warning and leaves statuses unchanged.
func (m *Manager) CheckCredentials(ctx context.Context) (CredentialReport, error) {
	m.locateGcloud()
	client := m.client()
	if !client.Available() {
		return CredentialReport{Warnings: []Warning{{Code: WarnGcloudNotFound, Message: gcloud.ErrNotFound.Error()}}}, nil
	}
	out, err := client.Query(ctx, credentialCheckTimeout, nil, "auth", "list", "--format=json")
	if err != nil {
		return CredentialReport{Warnings: []Warning{{Code: WarnCredentialCheck, Message: err.Error()}}}, nil
	}
	var entries []struct {
		Account string `json:"account"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return CredentialReport{Warnings: []Warning{{Code: WarnCredentialCheck, Message: fmt.Sprintf("unexpected gcloud auth list output: %v", err)}}}, nil
	}
	report := CredentialReport{}
	credentialed := map[string]bool{}
	for _, entry := range entries {
		report.Accounts = append(report.Accounts, entry.Account)
		// ACTIVE = currently active account with a usable token.
		// CREDENTIALED = has stored credentials (gcloud auth list still includes it).
		// Both mean gcloud has credentials on file; exclude anything else (e.g. empty).
		if strings.EqualFold(entry.Status, "ACTIVE") || strings.EqualFold(entry.Status, "CREDENTIALED") {
			credentialed[strings.ToLower(entry.Account)] = true
		}
	}
	m.stateMu.Lock()
	m.credentialed, m.credChecked = credentialed, true
	m.stateMu.Unlock()
	return report, nil
}
