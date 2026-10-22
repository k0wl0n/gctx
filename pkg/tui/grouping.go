package tui

import (
	"cmp"
	"slices"
	"strings"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

type sortKey int

const (
	sortName sortKey = iota
	sortStatus
	sortExpiry
)

func (s sortKey) String() string {
	return [...]string{"name", "status", "expiry"}[s]
}

// categoryOrder fixes the order of category groups; unknown categories sort last.
var categoryOrder = []provider.Category{provider.CategoryCloud, provider.CategoryAITool, provider.CategoryAIGateway}

func categoryRank(c provider.Category) int {
	if i := slices.Index(categoryOrder, c); i >= 0 {
		return i
	}
	return len(categoryOrder)
}

// categoryHeading is a category's list heading.
func categoryHeading(c provider.Category) string {
	switch c {
	case provider.CategoryCloud:
		return "CLOUDS"
	case provider.CategoryAITool:
		return "AI TOOLS"
	case provider.CategoryAIGateway:
		return "AI GATEWAYS"
	}
	return strings.ToUpper(string(c))
}

// listedProviders are the providers with accounts, by category then
// registry order. They make the tabs, header slots and list groups.
func (m Model) listedProviders() []manager.ProviderInfo {
	var out []manager.ProviderInfo
	for _, info := range m.overview.Providers {
		if info.Count > 0 {
			out = append(out, info)
		}
	}
	slices.SortStableFunc(out, func(a, b manager.ProviderInfo) int {
		return cmp.Compare(categoryRank(a.Category), categoryRank(b.Category))
	})
	return out
}

func (m Model) providerInfo(id provider.ID) manager.ProviderInfo {
	for _, info := range m.overview.Providers {
		if info.ID == id {
			return info
		}
	}
	return manager.ProviderInfo{ID: id, Label: string(id)}
}

// tabProvider is the provider of the current tab; ok is false on All.
func (m Model) tabProvider() (manager.ProviderInfo, bool) {
	listed := m.listedProviders()
	if m.tab <= 0 || m.tab > len(listed) {
		return manager.ProviderInfo{}, false
	}
	return listed[m.tab-1], true
}

func (m Model) tabCount() int { return len(m.listedProviders()) + 1 }

// listGroup is one provider's rows on the list, after every filter.
type listGroup struct {
	info   manager.ProviderInfo
	rows   []int // indices into overview.Accounts, in display order
	hidden int   // ready rows hidden by a fold
}

func needsAction(a manager.AccountStatus) bool { return a.Status != manager.HealthReady }

// statusOrder ranks health for the status sort: most urgent first.
var statusOrder = []manager.Health{
	manager.HealthError, manager.HealthCLIMissing, manager.HealthConfigMissing, manager.HealthAuthRequired,
	manager.HealthExpired, manager.HealthADCMissing, manager.HealthProjectMismatch, manager.HealthUnverified,
	manager.HealthBudgetLow, manager.HealthReady,
}

func healthRank(h manager.Health) int {
	if i := slices.Index(statusOrder, h); i >= 0 {
		return i
	}
	return len(statusOrder)
}

func (m Model) compareAccounts(a, b manager.AccountStatus) int {
	switch m.sort {
	case sortStatus:
		if c := cmp.Compare(healthRank(a.Status), healthRank(b.Status)); c != 0 {
			return c
		}
	case sortExpiry:
		switch {
		case a.ExpiresAt.IsZero() && !b.ExpiresAt.IsZero():
			return 1
		case !a.ExpiresAt.IsZero() && b.ExpiresAt.IsZero():
			return -1
		}
		if c := a.ExpiresAt.Compare(b.ExpiresAt); c != 0 {
			return c
		}
	}
	return cmp.Compare(a.Name, b.Name)
}

// buildGroups applies the tab, the query, the needs-action filter, the sort
// and the folds. Accounts of a provider missing from the overview's provider
// list still get a group, after the known ones.
func (m Model) buildGroups() []listGroup {
	tabInfo, onTab := m.tabProvider()
	query := strings.ToLower(m.filter.query)
	order := m.listedProviders()
	byID := map[provider.ID]*listGroup{}
	var groups []*listGroup
	group := func(id provider.ID) *listGroup {
		if g, ok := byID[id]; ok {
			return g
		}
		g := &listGroup{info: m.providerInfo(id)}
		byID[id] = g
		groups = append(groups, g)
		return g
	}
	for _, info := range order {
		group(info.ID)
	}
	for i, account := range m.overview.Accounts {
		if onTab && account.Provider != tabInfo.ID {
			continue
		}
		if query != "" && !matchesQuery(account, query) {
			continue
		}
		if m.needsAction && !needsAction(account) {
			continue
		}
		g := group(account.Provider)
		if m.folded[string(account.Provider)] && !needsAction(account) {
			g.hidden++
			continue
		}
		g.rows = append(g.rows, i)
	}
	var out []listGroup
	for _, g := range groups {
		if len(g.rows) == 0 && g.hidden == 0 {
			continue
		}
		slices.SortStableFunc(g.rows, func(a, b int) int {
			return m.compareAccounts(m.overview.Accounts[a], m.overview.Accounts[b])
		})
		out = append(out, *g)
	}
	return out
}
