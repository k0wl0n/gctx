package tui

import (
	tea "charm.land/bubbletea/v2"
)

// updateModal routes any message to the open modal. Stub.
func (m Model) updateModal(msg tea.Msg) (Model, tea.Cmd) {
	if m.modal == nil {
		return m, nil
	}
	return m.modal.update(m, msg)
}

// viewModal draws the open modal centred over base. Stub: returns base.
func (m Model) viewModal(base string) string { return base }

// handleOpDone applies the result of a background operation. Stub.
func (m Model) handleOpDone(msg opDoneMsg) (Model, tea.Cmd) { return m, nil }

// Action entry points called by the main-view key handler. Each takes the
// selected account name, except openCreate.
func (m Model) startSwitch(name string) (Model, tea.Cmd) { return m, nil }
func (m Model) openSave(name string) (Model, tea.Cmd)    { return m, nil }
func (m Model) openDelete(name string) (Model, tea.Cmd)  { return m, nil }
func (m Model) openCreate() (Model, tea.Cmd)             { return m, nil }

// openRun opens the run prompt (B3's newRunPrompt).
func (m Model) openRun(name string) (Model, tea.Cmd) {
	m.modal = newRunPrompt(name)
	return m, nil
}

// startShell and startLogin forward to the exec flows.
func (m Model) startShell(name string) (Model, tea.Cmd) { return m.beginShell(name) }
func (m Model) startLogin(name string) (Model, tea.Cmd) { return m.beginLogin(name) }
