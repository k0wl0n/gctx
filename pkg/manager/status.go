package manager

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

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
	if cfg, err := m.loadConfig(); err == nil && cfg.ActiveBySlot["gcp"] != "" {
		return cfg.ActiveBySlot["gcp"]
	}
	return "unknown"
}

// ActiveSlots returns the active account of every slot that has one. The
// gcp slot honours GCTX_ACCOUNT like ActiveAccount does.
func (m *Manager) ActiveSlots() map[string]string {
	slots := map[string]string{}
	m.mu.Lock()
	if cfg, err := m.loadConfig(); err == nil {
		for slot, name := range cfg.ActiveBySlot {
			slots[slot] = name
		}
	}
	m.mu.Unlock()
	if name := m.opts.Getenv("GCTX_ACCOUNT"); name != "" {
		slots["gcp"] = name
	}
	return slots
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
		ActiveAccount:   cfg.ActiveBySlot["gcp"],
		ActiveBySlot:    maps.Clone(cfg.ActiveBySlot),
		ShellAccount:    m.opts.Getenv("GCTX_ACCOUNT"),
		DefaultADCPath:  m.paths.DefaultADC,
		GcloudAvailable: clis[provider.GCP].Found,
	}
	cache := m.loadStatusCache()
	counts := map[provider.ID]int{}
	for _, account := range cfg.ListAccounts() {
		status := m.accountStatus(cfg, account, clis, cache)
		counts[status.Provider]++
		overview.Accounts = append(overview.Accounts, status)
	}
	overview.Providers = m.providerInfos(clis, counts)
	m.fillGlobalState(&overview)
	return overview, nil
}

// Providers describes every registered provider, in registry order, with
// the number of configured accounts. It reads files and PATH only.
func (m *Manager) Providers() []ProviderInfo {
	counts := map[provider.ID]int{}
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err == nil {
		for _, account := range cfg.ListAccounts() {
			counts[provider.ID(account.ProviderID())]++
		}
	}
	clis := map[provider.ID]provider.CLIStatus{}
	for _, p := range m.providers.All() {
		clis[p.ID()] = p.CLI()
	}
	return m.providerInfos(clis, counts)
}

func (m *Manager) providerInfos(clis map[provider.ID]provider.CLIStatus, counts map[provider.ID]int) []ProviderInfo {
	var infos []ProviderInfo
	for _, p := range m.providers.All() {
		envBased := false
		if marker, ok := p.(provider.EnvBased); ok {
			envBased = marker.EnvBased()
		}
		info := ProviderInfo{EnvBased: envBased, CanLogin: true, Form: p.Form(),
			ID: p.ID(), Label: p.Label(), Category: p.Category(), Badge: p.Badge(), Slot: p.Slot(),
			TargetLabel: p.TargetLabel(), Columns: p.Columns(), CLI: clis[p.ID()],
			CheckTimeout: m.opts.CredentialTimeout, Count: counts[p.ID()],
		}
		if footer, ok := p.(provider.FormFooter); ok {
			info.Footer = footer.Footer
		}
		if network, ok := p.(provider.NetworkAuth); ok {
			info.NetworkAuth = network.ChecksOverNetwork()
		}
		if installer, ok := p.(provider.Installer); ok {
			info.InstallHint = installer.InstallHint()
		}
		if login, ok := p.(provider.LoginCapable); ok {
			info.CanLogin = login.HasLogin()
		}
		if streaming, ok := p.(provider.StreamingLogin); ok {
			info.StreamsLogin = streaming.StreamsLogin()
		}
		infos = append(infos, info)
	}
	return infos
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
	return m.accountStatus(cfg, account, nil, m.loadStatusCache()), nil
}

// accountStatus computes one account's status. clis caches CLI lookups; nil
// looks the account's CLI up directly.
func (m *Manager) accountStatus(cfg *config.Config, account *config.Account, clis map[provider.ID]provider.CLIStatus, cache statusCache) AccountStatus {
	status := AccountStatus{
		Name:       account.Name,
		Provider:   provider.ID(account.ProviderID()),
		ConfigName: account.ConfigName,
		ProjectID:  account.ProjectID,
		Email:      account.Email,
		CreatedAt:  account.CreatedAt,
		ADCPath:    account.ADCPath,
		Settings:   maps.Clone(account.Settings),
	}
	p, err := m.providerFor(*account)
	if err != nil {
		status.Active = cfg.ActiveBySlot[account.ProviderID()] == account.Name
		status.Status = HealthError
		status.Findings = []Finding{{Health: HealthError, Message: err.Error()}}
		return status
	}
	status.Active = cfg.ActiveBySlot[p.Slot()] == account.Name
	status.TargetLabel = p.TargetLabel()
	status.Category = p.Category()
	status.Details = p.Details(*account)

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
		if inspected.ConfigExists && inspected.Local.Identity != "" {
			m.applyAuth(&status, p.ID(), inspected.Local.Identity, cache)
		}
	} else {
		local := p.Local(*account)
		status.Findings = local.Findings
		if len(status.Findings) == 0 && local.Status != "" && local.Status != HealthReady {
			status.Findings = []Finding{{Health: local.Status}}
		}
		m.applyAuth(&status, p.ID(), local.Identity, cache)
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
var findingOrder = []Health{
	HealthError, HealthCLIMissing, HealthConfigMissing, HealthAuthRequired, HealthExpired,
	HealthADCMissing, HealthProjectMismatch, HealthUnverified, HealthBudgetLow,
}

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

// applyAuth adds the account's last credential result to its status.
func (m *Manager) applyAuth(status *AccountStatus, id provider.ID, identity string, cache statusCache) {
	outcome, ok := m.authOutcome(id, status.Name, identity, cache)
	if !ok {
		return
	}
	status.Identity, status.ExpiresAt, status.Checked = outcome.Identity, outcome.ExpiresAt, outcome.Checked
	status.LastGoodAge, status.Budget = outcome.LastGoodAge, outcome.Budget
	if status.Identity == "" {
		status.Identity = identity
	}
	if outcome.Health != HealthReady && !status.HasFinding(outcome.Health) {
		status.Findings = insertFinding(status.Findings, Finding{Health: outcome.Health})
	}
}

// authOutcome resolves an account's last credential result: a failed
// re-authentication, then this session's check, then the status cache.
func (m *Manager) authOutcome(id provider.ID, name, identity string, cache statusCache) (authOutcome, bool) {
	var last *cachedResult
	if entry, ok := cache.Accounts[name]; ok {
		last = &entry
	}
	m.stateMu.Lock()
	reauth := m.reauth[name]
	res, found := m.sessionResult(id, name, identity)
	m.stateMu.Unlock()

	now := m.opts.Now()
	switch {
	case reauth:
		return authOutcome{Health: HealthAuthRequired}, true
	case found:
		return deriveHealth(res, last, now), true
	case last != nil:
		return deriveHealth(last.authResult(), nil, now), true
	}
	return authOutcome{}, false
}

// sessionResult finds this session's result for an account by name or
// identity, preferring a credentialed match. After a check that completed
// without warnings, an account it did not list is not credentialed. The
// caller holds stateMu.
func (m *Manager) sessionResult(id provider.ID, name, identity string) (provider.AuthResult, bool) {
	keys := []string{name}
	if identity != "" {
		keys = append(keys, strings.ToLower(identity))
	}
	// A name-keyed Err result comes from a check that timed out after any
	// identity-keyed result, so it wins.
	if res, ok := m.authResults[id][name]; ok && res.Err != nil {
		return res, true
	}
	var match provider.AuthResult
	found := false
	for _, key := range keys {
		res, ok := m.authResults[id][key]
		if !ok {
			continue
		}
		if res.Credentialed && res.Err == nil {
			return res, true
		}
		if !found {
			match, found = res, true
		}
	}
	if found {
		return match, true
	}
	if checked, ok := m.credChecked[id]; ok {
		return provider.AuthResult{Checked: checked}, true
	}
	return provider.AuthResult{}, false
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
	id        provider.ID
	accounts  []string
	report    provider.AuthReport
	abandoned bool // timed out; report holds an Err result for every account
}

// CheckCredentials asks every provider that has accounts which of them hold
// credentials, concurrently and each under its own timeout. A provider that
// fails yields a warning and leaves its accounts' statuses unchanged; one that
// times out also marks its accounts UNVERIFIED. The others' results are still
// applied.
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
	return m.applyChecks(cfg, results), nil
}

// CheckProviderCredentials is one provider's part of CheckCredentials, under
// that provider's timeout, so callers can report progress per provider.
func (m *Manager) CheckProviderCredentials(ctx context.Context, id provider.ID) (CredentialReport, error) {
	p, ok := m.providers.Get(id)
	if !ok {
		return CredentialReport{}, fmt.Errorf("unknown provider %q", id)
	}
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return CredentialReport{}, err
	}
	var accounts []config.Account
	for _, account := range cfg.ListAccounts() {
		if provider.ID(account.ProviderID()) == id {
			accounts = append(accounts, *account)
		}
	}
	if len(accounts) == 0 {
		return CredentialReport{}, nil
	}
	return m.applyChecks(cfg, []providerCheck{m.checkProvider(ctx, p, accounts)}), nil
}

// applyChecks records finished provider checks in the session state and the
// status cache.
func (m *Manager) applyChecks(cfg *config.Config, results []providerCheck) CredentialReport {
	now := m.opts.Now()
	var report CredentialReport
	m.stateMu.Lock()
	for _, result := range results {
		if m.networkFailed(result) {
			report.NetworkFailed = append(report.NetworkFailed, result.id)
		}
		stamped := make(map[string]provider.AuthResult, len(result.report.Results))
		for key, res := range result.report.Results {
			if res.Checked.IsZero() {
				res.Checked = now
			}
			stamped[key] = res
		}
		if len(result.report.Warnings) > 0 {
			// Per-account results of an incomplete check still apply (a timed-out
			// account becomes UNVERIFIED); unlisted accounts keep their status.
			report.Warnings = append(report.Warnings, result.report.Warnings...)
			if result.abandoned {
				// Drop earlier results, including identity-keyed ones, so none
				// outlives the timeout.
				m.authResults[result.id] = stamped
				continue
			}
			if m.authResults[result.id] == nil {
				m.authResults[result.id] = map[string]provider.AuthResult{}
			}
			maps.Copy(m.authResults[result.id], stamped)
			continue
		}
		m.authResults[result.id] = stamped
		m.credChecked[result.id] = now
		report.Accounts = append(report.Accounts, result.accounts...)
	}
	m.stateMu.Unlock()
	m.recordStatusCache(cfg, now)
	return report
}

// networkFailed reports whether a network-checked provider answered nothing:
// it had accounts checked and every one failed with a non-auth error (a
// timeout, an abandoned check or a CLI network error), none succeeding.
func (m *Manager) networkFailed(result providerCheck) bool {
	p, ok := m.providers.Get(result.id)
	if !ok {
		return false
	}
	if network, ok := p.(provider.NetworkAuth); !ok || !network.ChecksOverNetwork() || len(result.report.Results) == 0 {
		return false
	}
	for _, res := range result.report.Results {
		if res.Err == nil || res.Credentialed {
			return false
		}
	}
	return true
}

// recordStatusCache merges this session's per-account results into the
// status cache. The cache is best effort: a write failure is ignored.
func (m *Manager) recordStatusCache(cfg *config.Config, now time.Time) {
	path := m.paths.StatusCachePath()
	if path == "" {
		return
	}
	byName := map[string]provider.AuthResult{}
	for _, account := range cfg.ListAccounts() {
		p, err := m.providerFor(*account)
		if err != nil {
			continue
		}
		identity, applies := authIdentity(p, *account)
		if !applies {
			continue
		}
		m.stateMu.Lock()
		res, found := m.sessionResult(p.ID(), account.Name, identity)
		m.stateMu.Unlock()
		if found {
			byName[account.Name] = res
		}
	}

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	var cache statusCache
	cache.Load(path)
	cache.Merge(byName, now)
	for name := range cache.Accounts {
		if _, err := cfg.GetAccount(name); err != nil {
			delete(cache.Accounts, name)
		}
	}
	_ = cache.Save(path)
}

// forgetCachedStatus drops an account's cached result, after a login or
// delete made it stale.
func (m *Manager) forgetCachedStatus(name string) {
	path := m.paths.StatusCachePath()
	if path == "" {
		return
	}
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	var cache statusCache
	cache.Load(path)
	if _, ok := cache.Accounts[name]; !ok {
		return
	}
	delete(cache.Accounts, name)
	_ = cache.Save(path)
}

// loadStatusCache reads the status cache; it is empty when disabled, missing
// or corrupt.
func (m *Manager) loadStatusCache() statusCache {
	var cache statusCache
	if path := m.paths.StatusCachePath(); path != "" {
		cache.Load(path)
	}
	return cache
}

// authIdentity is the identity an account's credential result is matched by,
// and whether a result applies at all (see accountStatus). It reads files only.
func authIdentity(p provider.Provider, account config.Account) (string, bool) {
	if inspector, ok := p.(provider.StatusInspector); ok {
		inspected := inspector.Inspect(account)
		return inspected.Local.Identity, inspected.ConfigExists && inspected.Local.Identity != ""
	}
	return p.Local(account).Identity, true
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
				if res := result.report.Results[account.Name]; res.Credentialed && res.Err == nil {
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
		// An abandoned check is no verdict: every account gets a timeout result
		// and becomes UNVERIFIED, never AUTH REQUIRED.
		results := make(map[string]provider.AuthResult, len(accounts))
		for _, account := range accounts {
			results[account.Name] = provider.AuthResult{Err: ctx.Err()}
		}
		return providerCheck{id: p.ID(), abandoned: true, report: provider.AuthReport{Results: results, Warnings: []Warning{{
			Code:    WarnCredentialCheck,
			Message: fmt.Sprintf("%s credential check did not finish within %s: %v", p.Label(), timeout, ctx.Err()),
		}}}}
	}
}
