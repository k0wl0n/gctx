package manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/k0wl0n/gctx/pkg/provider"
)

// budgetLowFraction is the remaining share of a budget below which an account
// is BUDGET LOW.
const budgetLowFraction = 0.1

// authOutcome is what a credential result contributes to an account's status.
type authOutcome struct {
	Health      Health // READY when the result adds no finding
	Identity    string
	ExpiresAt   time.Time
	Checked     time.Time
	LastGoodAge time.Duration // UNVERIFIED with a cached last good result only
	Budget      *provider.Budget
}

// deriveHealth turns one credential result into a health verdict. A result
// carrying Err is no verdict: it is UNVERIFIED with the last good result, never
// AUTH REQUIRED. BUDGET LOW ranks below every other finding, so it shows only
// when the account is otherwise READY.
func deriveHealth(res provider.AuthResult, last *cachedResult, now time.Time) authOutcome {
	if res.Err != nil {
		outcome := authOutcome{Health: HealthUnverified, Identity: res.Identity, Checked: res.Checked}
		if last != nil {
			outcome.Identity, outcome.ExpiresAt, outcome.Budget = last.Identity, last.ExpiresAt, last.Budget
			outcome.Checked, outcome.LastGoodAge = last.Checked, now.Sub(last.Checked)
		}
		return outcome
	}
	outcome := authOutcome{Health: HealthReady, Identity: res.Identity, ExpiresAt: res.ExpiresAt, Checked: res.Checked, Budget: res.Budget}
	switch {
	case !res.Credentialed:
		outcome.Health = HealthAuthRequired
	case !res.ExpiresAt.IsZero() && !res.ExpiresAt.After(now):
		outcome.Health = HealthExpired
	case res.Budget != nil && res.Budget.Limit > 0 && res.Budget.Limit-res.Budget.Spent < budgetLowFraction*res.Budget.Limit:
		outcome.Health = HealthBudgetLow
	}
	return outcome
}

// cachedResult is one account's last completed credential check. It holds no
// token, key or secret: only what the status line shows.
type cachedResult struct {
	Health    Health           `json:"health"`
	Identity  string           `json:"identity,omitempty"`
	ExpiresAt time.Time        `json:"expires_at,omitzero"`
	Checked   time.Time        `json:"checked"`
	Budget    *provider.Budget `json:"budget,omitempty"`
}

// authResult rebuilds the result the entry was cached from.
func (c cachedResult) authResult() provider.AuthResult {
	return provider.AuthResult{
		Credentialed: c.Health != HealthAuthRequired,
		Identity:     c.Identity,
		ExpiresAt:    c.ExpiresAt,
		Checked:      c.Checked,
		Budget:       c.Budget,
	}
}

// statusCache is the on-disk record of last results, so the first paint after
// launch is not blank. A missing or corrupt file is an empty cache.
type statusCache struct {
	Accounts map[string]cachedResult `json:"accounts"`
}

func (c *statusCache) Load(path string) {
	c.Accounts = nil
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var loaded statusCache
	if json.Unmarshal(data, &loaded) == nil {
		c.Accounts = loaded.Accounts
	}
}

// Save writes the cache atomically, readable by the owner only.
func (c *statusCache) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".status-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Merge records results keyed by account name. A result carrying Err keeps
// the previous entry, so the cache always holds the last good result.
func (c *statusCache) Merge(report map[string]provider.AuthResult, now time.Time) {
	for name, res := range report {
		if res.Err != nil {
			continue
		}
		if res.Checked.IsZero() {
			res.Checked = now
		}
		outcome := deriveHealth(res, nil, now)
		if c.Accounts == nil {
			c.Accounts = map[string]cachedResult{}
		}
		c.Accounts[name] = cachedResult{
			Health:    outcome.Health,
			Identity:  outcome.Identity,
			ExpiresAt: outcome.ExpiresAt,
			Checked:   outcome.Checked,
			Budget:    outcome.Budget,
		}
	}
}
