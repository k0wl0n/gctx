package manager

import (
	"context"
	"os"
	"strings"

	"github.com/k0wl0n/gctx/pkg/adc"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloudconfig"
)

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
	m.gcp.Locate()
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
	inspected := m.gcp.Inspect(*account)
	status := AccountStatus{
		Name:               account.Name,
		ConfigName:         account.ConfigName,
		ProjectID:          account.ProjectID,
		Email:              account.Email,
		CreatedAt:          account.CreatedAt,
		Active:             account.Name == cfg.ActiveAccount,
		ADCPath:            account.ADCPath,
		StoredADCPath:      inspected.StoredADCPath,
		ADCExists:          inspected.ADCExists,
		ADCModTime:         inspected.ADCModTime,
		RecordedADCModTime: inspected.RecordedADCModTime,
		GcloudConfigExists: inspected.ConfigExists,
		GcloudAccount:      inspected.ConfigAccount,
		GcloudProject:      inspected.ConfigProject,
		Findings:           inspected.Local.Findings,
	}
	if inspected.ConfigExists && inspected.Local.Identity != "" && m.authRequired(account.Name, inspected.Local.Identity) {
		status.Findings = insertFinding(status.Findings, Finding{Health: HealthAuthRequired})
	}
	status.Status = HealthReady
	if len(status.Findings) > 0 {
		status.Status = status.Findings[0].Health
	}
	return status
}

// findingOrder ranks findings; the first finding is the account's status.
var findingOrder = []Health{HealthError, HealthConfigMissing, HealthAuthRequired, HealthADCMissing, HealthProjectMismatch}

func findingRank(h Health) int {
	for rank, candidate := range findingOrder {
		if candidate == h {
			return rank
		}
	}
	return len(findingOrder)
}

// insertFinding places finding after every finding of equal or higher priority.
func insertFinding(findings []Finding, finding Finding) []Finding {
	position := len(findings)
	for i, existing := range findings {
		if findingRank(existing.Health) > findingRank(finding.Health) {
			position = i
			break
		}
	}
	return append(findings[:position:position], append([]Finding{finding}, findings[position:]...)...)
}

// authRequired applies the session state: a failed re-authentication or a
// completed credential check that did not list the gcloud account.
func (m *Manager) authRequired(name, gcloudAccount string) bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.reauth[name] || (m.credChecked && !m.credentialed[strings.ToLower(gcloudAccount)])
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
	accounts, auth := m.gcp.ListAuth(ctx)
	if len(auth.Warnings) > 0 {
		return CredentialReport{Warnings: auth.Warnings}, nil
	}
	m.stateMu.Lock()
	m.credentialed, m.credChecked = auth.Credentialed, true
	m.stateMu.Unlock()
	return CredentialReport{Accounts: accounts}, nil
}
