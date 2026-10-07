package tui

import tea "charm.land/bubbletea/v2"

// updateMain handles keys in the main view. Stub: quit only.
func (m Model) updateMain(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if keys.Quit.Enabled() && matches(msg, keys.Quit.Keys()) {
		return m, tea.Quit
	}
	return m, nil
}

// updateFilter handles keys while the filter input has focus. Stub.
func (m Model) updateFilter(msg tea.KeyPressMsg) (Model, tea.Cmd) { return m, nil }

// updateOverlay handles keys on the help and details screens. Stub.
func (m Model) updateOverlay(msg tea.KeyPressMsg) (Model, tea.Cmd) { return m, nil }

// handleCreds merges a credential check result. Stub.
func (m Model) handleCreds(msg credsCheckedMsg) (Model, tea.Cmd) { return m, nil }

func matches(msg tea.KeyPressMsg, bound []string) bool {
	for _, k := range bound {
		if msg.String() == k {
			return true
		}
	}
	return false
}
