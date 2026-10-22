package tui

import (
	"charm.land/bubbles/v2/key"
	"maps"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func matches(msg tea.KeyPressMsg, bound []string) bool {
	for _, k := range bound {
		if msg.String() == k {
			return true
		}
	}
	return false
}

// updateMain handles all key presses in the main view.
func (m Model) updateMain(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := msg.String()

	// quit always works
	if matches(msg, keys.Quit.Keys()) {
		if m.busy() {
			if k == "ctrl+c" {
				if !m.quitArmed.IsZero() && m.now().Before(m.quitArmed.Add(quitConfirmWindow)) {
					return m, tea.Quit
				}
				m.quitArmed = m.now()
				return m, m.setStatus("Operation in progress — press ctrl+c again within 3s to quit", levelWarning)
			}
			return m, m.setStatus("Busy: "+m.op.desc+" — wait for it to finish, or press ctrl+c twice to force quit", levelWarning)
		}
		return m, tea.Quit
	}

	if m.feedback != nil && dismissesFeedback(msg) {
		m.feedback = nil
	}

	// navigation is always available (not gated by busy)
	switch {
	case matches(msg, keys.Down.Keys()):
		m = m.moveSelection(1)
		return m, nil
	case matches(msg, keys.Up.Keys()):
		m = m.moveSelection(-1)
		return m, nil
	case matches(msg, keys.First.Keys()):
		m = m.moveToIndex(0)
		return m, nil
	case matches(msg, keys.Last.Keys()):
		m = m.moveToIndex(len(m.nav) - 1)
		return m, nil
	case matches(msg, keys.PageDown.Keys()):
		m = m.moveSelection(m.pageSize())
		return m, nil
	case matches(msg, keys.PageUp.Keys()):
		m = m.moveSelection(-m.pageSize())
		return m, nil
	case matches(msg, keys.Help.Keys()):
		m.screen = screenHelp
		return m, nil
	case matches(msg, keys.Details.Keys()):
		m.screen = screenDetails
		return m, nil
	case matches(msg, keys.Filter.Keys()):
		m.filter.editing = true
		m.filter.input.SetValue(m.filter.query)
		return m, nil
	case matches(msg, keys.Clear.Keys()):
		if m.feedback != nil {
			m.feedback = nil
			return m, nil
		}
		m.filter.query = ""
		m.filter.input.Reset()
		m.rebuildVisible()
		return m, nil
	case matches(msg, keys.NextTab.Keys()):
		return m.selectTab((m.tab + 1) % m.tabCount()), nil
	case matches(msg, keys.PrevTab.Keys()):
		return m.selectTab((m.tab + m.tabCount() - 1) % m.tabCount()), nil
	case matches(msg, keys.JumpTab.Keys()):
		if n, err := strconv.Atoi(k); err == nil && n <= m.tabCount() {
			return m.selectTab(n - 1), nil
		}
		return m, nil
	case matches(msg, keys.TogglePane.Keys()):
		m.paneOpen = !m.paneOpen
		if tierFor(m.width) != tierWide {
			return m, m.setStatus("The detail pane needs 150 columns; d opens details", levelInfo)
		}
		return m, nil
	case matches(msg, keys.Fold.Keys()):
		return m.toggleFold()
	case matches(msg, keys.NeedsAction.Keys()):
		m.needsAction = !m.needsAction
		m.refilter()
		return m, nil
	case matches(msg, keys.Sort.Keys()):
		m.sort = (m.sort + 1) % (sortExpiry + 1)
		m.rebuildVisible() // the tab row shows the sort
		return m, nil
	case matches(msg, keys.Reload.Keys()):
		m.reload()
		cmd := m.startCredsCheck()
		return m, tea.Batch(cmd, m.loadEnvCmd())
	case matches(msg, keys.Command.Keys()):
		return m, m.setStatus("Command mode is not available yet", levelInfo)
	}

	if _, ok := m.selectedPlaceholder(); ok && accountKeys[k] {
		return m, m.setStatus("This group is folded: press z to expand it", levelInfo)
	}

	if matches(msg, keys.CopyEnv.Keys()) {
		return m.copyEnvLine()
	}

	// actions below may be gated by busy
	if m.busy() {
		return m, m.setStatus("Busy: "+m.op.desc, levelWarning)
	}

	// CLI gating: switch, login and run need the selected row's provider CLI;
	// create still needs gcloud while the create form is GCP-only.
	acct, hasSelection := m.selectedAccount()
	if m.feedback != nil && hasSelection {
		acct.Name = m.feedback.account // the panel's keys act on what it shows
	}
	if cliGated[k] && hasSelection {
		if info, known := m.knownProvider(acct.Provider); known && !info.CLI.Found {
			return m, m.setStatus(cliMissingText(info), levelError)
		}
	}
	if k == "c" && !m.overview.GcloudAvailable {
		return m, m.setStatus("gcloud not found — install it and press R to reload", levelError)
	}

	// selection-required actions
	noSelectionNeeded := map[string]bool{"c": true}

	if !noSelectionNeeded[k] && !hasSelection && len(m.visible) == 0 {
		return m, m.setStatus("No account selected", levelInfo)
	}

	switch k {
	case "enter":
		return m.startSwitch(acct.Name)
	case "a":
		if !hasSelection {
			return m, m.setStatus("No account selected", levelInfo)
		}
		if !adcApplies(acct) {
			return m, nil // saving ADC exists only for accounts with ADC
		}
		return m.openSave(acct.Name)
	case "ctrl+d":
		if !hasSelection {
			return m, m.setStatus("No account selected", levelInfo)
		}
		return m.openDelete(acct.Name)
	case "c":
		return m.openCreate()
	case "r":
		if !hasSelection {
			return m, m.setStatus("No account selected", levelInfo)
		}
		return m.openRun(acct.Name)
	case "s":
		if !hasSelection {
			return m, m.setStatus("No account selected", levelInfo)
		}
		return m.startShell(acct.Name, manager.ShellAllActive)
	case keys.ShellOne.Keys()[0]:
		if !hasSelection {
			return m, m.setStatus("No account selected", levelInfo)
		}
		return m.startShell(acct.Name, manager.ShellOnly)
	case "l":
		if !hasSelection {
			return m, m.setStatus("No account selected", levelInfo)
		}
		return m.startLogin(acct.Name)
	}

	return m, nil
}

// updateFilter handles key presses when the filter input is active.
func (m Model) updateFilter(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "enter":
		m.filter.editing = false
		m.filter.query = m.filter.input.Value()
		m.rebuildVisible()
		return m, nil
	case "esc":
		m.filter.editing = false
		m.filter.query = ""
		m.filter.input.Reset()
		m.rebuildVisible()
		return m, nil
	case "up":
		m = m.moveSelection(-1)
		return m, nil
	case "down":
		m = m.moveSelection(1)
		return m, nil
	default:
		m.filter.input.update(msg)
		// live filter as user types
		m.filter.query = m.filter.input.Value()
		m.rebuildVisible()
		return m, nil
	}
}

// updateOverlay handles key presses on help and details screens.
func (m Model) updateOverlay(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "esc", "q", "d", "i", "?":
		m.screen = screenMain
		return m, nil
	}
	return m, nil
}

func (m Model) selectTab(tab int) Model {
	m.tab = tab
	m.refilter()
	return m
}

// accountKeys act on the selected account; a placeholder is not one.
var accountKeys = map[string]bool{"enter": true, "a": true, "ctrl+d": true, "r": true, "s": true, "S": true, "y": true, "l": true}

var cliGated = map[string]bool{"enter": true, "l": true, "r": true}

// knownProvider is the provider's metadata when the overview lists it.
func (m Model) knownProvider(id provider.ID) (manager.ProviderInfo, bool) {
	for _, info := range m.overview.Providers {
		if info.ID == id {
			return info, true
		}
	}
	return manager.ProviderInfo{}, false
}

func cliMissingText(info manager.ProviderInfo) string {
	name := info.CLI.Name
	if name == "" {
		name = info.Label + " CLI"
	}
	return name + " not found — install it and press R to reload"
}

// toggleFold folds or unfolds the group of the selected row or placeholder.
// Folding moves the cursor to the placeholder when its row is hidden;
// unfolding moves it to the group's first row.
func (m Model) toggleFold() (Model, tea.Cmd) {
	var id provider.ID
	if g, ok := m.selectedPlaceholder(); ok {
		id = g.info.ID
	} else if acct, ok := m.selectedAccount(); ok {
		id = acct.Provider
	} else {
		return m, nil
	}
	m.folded = cloneOrNew(m.folded)
	if m.folded[string(id)] {
		delete(m.folded, string(id))
	} else {
		m.folded[string(id)] = true
	}
	previous := m.selected
	m.refilter()
	switch {
	case !m.folded[string(id)]:
		for _, g := range m.groups {
			if g.info.ID == id && len(g.rows) > 0 {
				m.selected = m.overview.Accounts[g.rows[0]].Name
			}
		}
	case !m.isVisible(previous) && m.isVisible(foldKey(id)):
		m.selected = foldKey(id)
	}
	return m, nil
}

// handleCreds merges one provider's (or a whole) credential check result.
// The check is done when every provider has reported.
func (m Model) handleCreds(msg credsCheckedMsg) (Model, tea.Cmd) {
	if msg.seq != m.creds.seq {
		return m, nil // stale
	}
	if msg.provider == "" {
		m.creds.progress = nil
		if msg.err != nil {
			m.creds.phase = credFailed
			m.creds.err = msg.err
			return m, nil
		}
	} else {
		// Models are values: copy the map so earlier models stay unchanged.
		m.creds.progress = cloneOrNew(m.creds.progress)
		m.creds.progress[msg.provider] = finishedProgress(m.creds.progress[msg.provider], msg)
	}
	for _, p := range m.creds.progress {
		if !p.done {
			m.reload()
			return m, nil
		}
	}
	m.creds.phase = credDone
	m.creds.finished = m.now()
	m.reload()
	return m, nil
}

func cloneOrNew[K comparable, V any](in map[K]V) map[K]V {
	if out := maps.Clone(in); out != nil {
		return out
	}
	return map[K]V{}
}

func finishedProgress(p checkProgress, msg credsCheckedMsg) checkProgress {
	p.done = true
	switch {
	case msg.err != nil:
		p.failed, p.warning = true, msg.err.Error()
	case len(msg.report.Warnings) > 0:
		p.warning = msg.report.Warnings[0].Summary()
		p.checked = len(msg.report.Accounts)
	default:
		p.checked = p.total
	}
	return p
}

// moveSelection moves by delta rows, clamping at the ends (no wrap).
func (m Model) moveSelection(delta int) Model {
	if len(m.nav) == 0 {
		return m
	}
	idx := m.selectedIndex() + delta
	return m.moveToIndex(idx)
}

// moveToIndex selects the row at the given index, clamped.
func (m Model) moveToIndex(idx int) Model {
	if len(m.nav) == 0 {
		return m
	}
	idx = max(min(idx, len(m.nav)-1), 0)
	m.selected = m.rowKey(m.nav[idx])
	// adjust scroll offset
	bodyLines := m.height - 5
	if bodyLines < 1 {
		bodyLines = 1
	}
	if idx < m.offset {
		m.offset = idx
	}
	if idx >= m.offset+bodyLines {
		m.offset = idx - bodyLines + 1
	}
	return m
}

func (m Model) pageSize() int {
	ps := m.height - 5
	if ps < 1 {
		return 1
	}
	return ps
}

// dismissesFeedback reports keys that move the selection or change what is
// listed, so the panel never describes a row other than the one acted on.
func dismissesFeedback(msg tea.KeyPressMsg) bool {
	for _, binding := range []key.Binding{
		keys.Down, keys.Up, keys.First, keys.Last, keys.PageDown, keys.PageUp,
		keys.NextTab, keys.PrevTab, keys.JumpTab, keys.Filter, keys.Fold, keys.NeedsAction, keys.Sort,
	} {
		if matches(msg, binding.Keys()) {
			return true
		}
	}
	return false
}

// copyEnvLine copies the eval line of the selected account (or the account
// the open panel shows). It is a lookup command, never a secret. Providers
// that switch globally have nothing to copy.
func (m Model) copyEnvLine() (Model, tea.Cmd) {
	acct, ok := m.selectedAccount()
	if !ok {
		return m, m.setStatus("No account selected", levelInfo)
	}
	if m.feedback != nil {
		acct.Name = m.feedback.account
	}
	if info := m.providerInfo(acct.Provider); !info.EnvBased {
		return m, m.setStatus(info.Label+" switches globally; nothing to copy", levelInfo)
	}
	line := envEvalLine(acct.Name)
	return m, tea.Batch(tea.SetClipboard(line), m.setStatus("copied: "+line, levelInfo))
}
