package tui

import (
	"maps"
	"strconv"

	tea "charm.land/bubbletea/v2"
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
		m = m.moveToIndex(len(m.visible) - 1)
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

	// actions below may be gated by busy
	if m.busy() {
		return m, m.setStatus("Busy: "+m.op.desc, levelWarning)
	}

	// gcloud-unavailable gating (task 7.4): some actions need gcloud
	gcloudGated := map[string]bool{
		"enter": true,
		"l":     true,
		"r":     true,
		"c":     true,
	}
	if gcloudGated[k] && !m.overview.GcloudAvailable {
		return m, m.setStatus("gcloud not found — install it and press R to reload", levelError)
	}

	// selection-required actions
	acct, hasSelection := m.selectedAccount()
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
		return m.startShell(acct.Name)
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

// toggleFold folds or unfolds the selected row's provider group on the All tab.
func (m Model) toggleFold() (Model, tea.Cmd) {
	if _, onTab := m.tabProvider(); onTab {
		return m, m.setStatus("Folding works on the All tab", levelInfo)
	}
	acct, ok := m.selectedAccount()
	if !ok {
		return m, nil
	}
	id := string(acct.Provider)
	m.folded = cloneOrNew(m.folded)
	m.folded[id] = !m.folded[id]
	if !m.folded[id] {
		delete(m.folded, id)
	}
	m.refilter()
	// Stay in the folded group when it still shows rows.
	if !m.isVisible(acct.Name) {
		for _, g := range m.groups {
			if string(g.info.ID) == id && len(g.rows) > 0 {
				m.selected = m.overview.Accounts[g.rows[0]].Name
			}
		}
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
	if len(m.visible) == 0 {
		return m
	}
	idx := m.selectedIndex() + delta
	return m.moveToIndex(idx)
}

// moveToIndex selects the row at the given index, clamped.
func (m Model) moveToIndex(idx int) Model {
	if len(m.visible) == 0 {
		return m
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.visible) {
		idx = len(m.visible) - 1
	}
	m.selected = m.overview.Accounts[m.visible[idx]].Name
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
