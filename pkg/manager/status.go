package manager

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
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

// Overview computes the local status of every account. It reads files and
// looks CLIs up on PATH only: no subprocess, no network, and no writes.
func (m *Manager) Overview() (Overview, error) {
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return Overview{}, err
	}

	clis := map[provider.ID]provider.CLIStatus{}
	for _, p := range m.providers.All() {
		clis[p.ID()] = p.CLI()
	}
	overview := Overview{
		ConfigPath:      m.paths.ConfigPath(),
		ActiveAccount:   cfg.ActiveAccount,
		ShellAccount:    m.opts.Getenv("GCTX_ACCOUNT"),
		DefaultADCPath:  m.paths.DefaultADC,
		GcloudAvailable: clis[provider.GCP].Found,
	}
	counts := map[provider.ID]int{}
	for _, account := range cfg.ListAccounts() {
		status := m.accountStatus(cfg, account, clis)
		counts[status.Provider]++
		overview.Accounts = append(overview.Accounts, status)
	}
	for _, p := range m.providers.All() {
		overview.Providers = append(overview.Providers, ProviderInfo{ID: p.ID(), Label: p.Label(), CLI: clis[p.ID()], Count: counts[p.ID()]})
	}
	m.fillGlobalState(&overview)
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
	return m.accountStatus(cfg, account, nil), nil
}

// accountStatus computes one account's status. clis caches CLI lookups; nil
// looks the account's CLI up directly.
func (m *Manager) accountStatus(cfg *config.Config, account *config.Account, clis map[provider.ID]provider.CLIStatus) AccountStatus {
	status := AccountStatus{
		Name:       account.Name,
		Provider:   provider.ID(account.ProviderID()),
		ConfigName: account.ConfigName,
		ProjectID:  account.ProjectID,
		Email:      account.Email,
		CreatedAt:  account.CreatedAt,
		Active:     account.Name == cfg.ActiveAccount,
		ADCPath:    account.ADCPath,
	}
	p, err := m.providerFor(*account)
	if err != nil {
		status.Status = HealthError
		status.Findings = []Finding{{Health: HealthError, Message: err.Error()}}
		return status
	}
	status.TargetLabel = p.TargetLabel()

	if inspector, ok := p.(provider.StatusInspector); ok {
		inspected := inspector.Inspect(*account)
		status.StoredADCPath = inspected.StoredADCPath
		status.ADCExists = inspected.ADCExists
		status.ADCModTime = inspected.ADCModTime
		status.RecordedADCModTime = inspected.RecordedADCModTime
		status.GcloudConfigExists = inspected.ConfigExists
		status.GcloudAccount = inspected.ConfigAccount
		status.GcloudProject = inspected.ConfigProject
		status.Findings = inspected.Local.Findings
		// Without a configured identity the inspector already reports AUTH REQUIRED.
		if inspected.ConfigExists && inspected.Local.Identity != "" && m.authRequired(p.ID(), account.Name, inspected.Local.Identity) {
			status.Findings = insertFinding(status.Findings, Finding{Health: HealthAuthRequired})
		}
	} else {
		local := p.Local(*account)
		status.Findings = local.Findings
		if len(status.Findings) == 0 && local.Status != "" && local.Status != HealthReady {
			status.Findings = []Finding{{Health: local.Status}}
		}
		if !status.HasFinding(HealthAuthRequired) && m.authRequired(p.ID(), account.Name, local.Identity) {
			status.Findings = insertFinding(status.Findings, Finding{Health: HealthAuthRequired})
		}
	}

	cli, cached := clis[p.ID()]
	if !cached {
		cli = p.CLI()
	}
	// GCP keeps its pre-multi-cloud behavior: a missing gcloud does not change
	// account status, because ADC files stay usable without it.
	if !cli.Found && p.ID() != provider.GCP {
		status.Findings = insertFinding(status.Findings, Finding{Health: HealthCLIMissing})
	}

	status.Status = HealthReady
	if len(status.Findings) > 0 {
		status.Status = status.Findings[0].Health
	}
	return status
}

// findingOrder ranks findings; the first finding is the account's status.
var findingOrder = []Health{HealthError, HealthCLIMissing, HealthConfigMissing, HealthAuthRequired, HealthADCMissing, HealthProjectMismatch}

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
// completed credential check for the provider that listed neither the
// account name nor its identity.
func (m *Manager) authRequired(id provider.ID, name, identity string) bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if m.reauth[name] {
		return true
	}
	if !m.credChecked[id] {
		return false
	}
	credentialed := m.credentialed[id]
	return !credentialed[name] && (identity == "" || !credentialed[strings.ToLower(identity)])
}

// fillGlobalState fills the gcloud active configuration and the default ADC
// comparison from providers that have global state.
func (m *Manager) fillGlobalState(overview *Overview) {
	overview.DefaultADC = ADCNone
	for _, p := range m.providers.All() {
		reader, ok := p.(provider.GlobalStateReader)
		if !ok {
			continue
		}
		stored := ""
		if active, ok := overview.Active(); ok && active.Provider == p.ID() {
			stored = active.StoredADCPath
		}
		state := reader.GlobalState(stored)
		overview.GcloudActiveConfig = state.ActiveConfig
		overview.DefaultADCPath = state.DefaultCredentialPath
		if !state.DefaultCredentialExists {
			return
		}
		overview.DefaultADCExists, overview.DefaultADCModTime = true, state.DefaultCredentialModTime
		switch {
		case !state.Compared:
			overview.DefaultADC = ADCUnknown
		case state.Matches:
			overview.DefaultADC = ADCActive
		default:
			overview.DefaultADC = ADCDiffers
		}
		return
	}
}

// providerCheck is one provider's part of CheckCredentials.
type providerCheck struct {
	id       provider.ID
	accounts []string
	report   provider.AuthReport
}

// CheckCredentials asks every provider that has accounts which of them hold
// credentials, concurrently and each under its own timeout. A provider that
// fails or times out yields a warning and leaves its accounts' statuses
// unchanged; the others' results are still applied.
func (m *Manager) CheckCredentials(ctx context.Context) (CredentialReport, error) {
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return CredentialReport{}, err
	}
	byProvider := map[provider.ID][]config.Account{}
	for _, account := range cfg.ListAccounts() {
		id := provider.ID(account.ProviderID())
		byProvider[id] = append(byProvider[id], *account)
	}

	var checked []provider.Provider
	for _, p := range m.providers.All() {
		if len(byProvider[p.ID()]) > 0 {
			checked = append(checked, p)
		}
	}
	results := make([]providerCheck, len(checked))
	var wg sync.WaitGroup
	for i, p := range checked {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = m.checkProvider(ctx, p, byProvider[p.ID()])
		}()
	}
	wg.Wait()

	var report CredentialReport
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	for _, result := range results {
		if len(result.report.Warnings) > 0 {
			report.Warnings = append(report.Warnings, result.report.Warnings...)
			continue
		}
		m.credentialed[result.id] = result.report.Credentialed
		m.credChecked[result.id] = true
		report.Accounts = append(report.Accounts, result.accounts...)
	}
	return report, nil
}

// checkProvider runs one provider's check, abandoning it at the timeout. An
// abandoned check finishes in the background and its result is discarded.
func (m *Manager) checkProvider(ctx context.Context, p provider.Provider, accounts []config.Account) providerCheck {
	timeout := m.opts.CredentialTimeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan providerCheck, 1)
	go func() {
		result := providerCheck{id: p.ID()}
		if lister, ok := p.(provider.AuthLister); ok {
			result.accounts, result.report = lister.ListAuth(ctx)
		} else {
			result.report = p.CheckAuth(ctx, accounts)
			for _, account := range accounts {
				if result.report.Credentialed[account.Name] {
					result.accounts = append(result.accounts, account.Name)
				}
			}
		}
		done <- result
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		return providerCheck{id: p.ID(), report: provider.AuthReport{Warnings: []Warning{{
			Code:    WarnCredentialCheck,
			Message: fmt.Sprintf("%s credential check did not finish within %s: %v", p.Label(), timeout, ctx.Err()),
		}}}}
	}
}
