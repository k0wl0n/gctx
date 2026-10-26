package manager

import (
	"fmt"
	"slices"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// linkedName is the account acct runs on, or "" when its provider does not
// link accounts or acct has its own login (D12).
func linkedName(p provider.Provider, acct config.Account) string {
	if _, ok := p.(provider.Linker); !ok {
		return ""
	}
	return acct.Settings[provider.SettingVia]
}

// linkTarget resolves the account acct runs on: it must exist, be a cloud
// account and not itself run on another account.
func (m *Manager) linkTarget(cfg *config.Config, acct config.Account, via string) (config.Account, provider.Provider, error) {
	linked, ok := cfg.Accounts[via]
	if !ok || via == acct.Name {
		return config.Account{}, nil, fmt.Errorf("%s runs on %q, which is not an account", acct.Name, via)
	}
	lp, err := m.providerFor(*linked)
	if err != nil {
		return config.Account{}, nil, err
	}
	if lp.Category() != provider.CategoryCloud {
		return config.Account{}, nil, fmt.Errorf("%s runs on %s, which is not a cloud account", acct.Name, via)
	}
	return *linked, lp, nil
}

// validateLink checks a new account's settings.via before it is created.
func (m *Manager) validateLink(cfg *config.Config, p provider.Provider, acct config.Account) error {
	linker, ok := p.(provider.Linker)
	via := acct.Settings[provider.SettingVia]
	if !ok || via == "" {
		return nil
	}
	linked, lp, err := m.linkTarget(cfg, acct, via)
	if err != nil {
		return err
	}
	_, err = linker.LinkEnv(acct, linked, lp.ID())
	return err
}

// accountEnv is the account's own env, or for a linked account the linked
// account's env followed by the tool's env for that backend.
func (m *Manager) accountEnv(p provider.Provider, acct config.Account) ([]string, error) {
	via := linkedName(p, acct)
	if via == "" {
		return p.Env(acct)
	}
	m.mu.Lock()
	cfg, err := m.loadConfig()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	linked, lp, err := m.linkTarget(cfg, acct, via)
	if err != nil {
		return nil, err
	}
	linkedEnv, err := lp.Env(linked)
	if err != nil {
		return nil, err
	}
	toolEnv, err := p.(provider.Linker).LinkEnv(acct, linked, lp.ID())
	if err != nil {
		return nil, err
	}
	return append(slices.Clone(linkedEnv), toolEnv...), nil
}

// inheritLinked replaces a linked account's health with the health of the
// account it runs on; an invalid link is an ERROR.
func (m *Manager) inheritLinked(cfg *config.Config, p provider.Provider, account *config.Account, status *AccountStatus, clis map[provider.ID]provider.CLIStatus, cache statusCache) {
	status.Linked = account.Settings[provider.SettingVia]
	linked, lp, err := m.linkTarget(cfg, *account, status.Linked)
	if err == nil {
		_, err = p.(provider.Linker).LinkEnv(*account, linked, lp.ID())
	}
	if err != nil {
		status.Findings = []Finding{{Health: HealthError, Message: err.Error()}}
		return
	}
	linkedStatus := m.accountStatus(cfg, &linked, clis, cache)
	status.Findings = slices.Clone(linkedStatus.Findings)
	if len(status.Findings) == 0 && linkedStatus.Status != HealthReady && linkedStatus.Status != "" {
		status.Findings = []Finding{{Health: linkedStatus.Status}}
	}
	status.Identity, status.ExpiresAt, status.Checked = linkedStatus.Identity, linkedStatus.ExpiresAt, linkedStatus.Checked
	status.LastGoodAge = linkedStatus.LastGoodAge
}
