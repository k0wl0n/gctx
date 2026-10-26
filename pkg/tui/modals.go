package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// warningMessage maps a warning code to a short user-facing message.
func warningMessage(w manager.Warning) string {
	switch w.Code {
	case manager.WarnADCMissing:
		return "ADC not saved: no credentials file to copy"
	case manager.WarnReauthRequired:
		return "re-authentication required"
	case manager.WarnSetProjectFailed:
		return "project could not be set"
	case manager.WarnConfigReused:
		return "gcloud config already existed; re-used"
	case manager.WarnConfigDeleteFailed:
		return "gcloud config deletion failed"
	case manager.WarnADCRemoveFailed:
		return "ADC removal failed"
	case manager.WarnADCPathOutsideStore:
		return "ADC path is outside the ADC store"
	case manager.WarnADCNotRefreshed:
		return "ADC was not refreshed"
	default:
		return w.Summary()
	}
}

// startSwitch begins a background switch to name, guarding against concurrent ops.
func (m Model) startSwitch(name string) (Model, tea.Cmd) {
	if m.busy() {
		return m, m.setStatus(fmt.Sprintf("Busy: %s", m.op.desc), levelWarning)
	}
	m.seq++
	seq := m.seq
	m.feedback = nil
	m.op = &operation{kind: opSwitch, account: name, desc: "switching", seq: seq, started: m.now()}
	if m.debugLog != nil {
		m.debugLog.Info("op", "kind", opSwitch, "account", name)
	}
	cmd := func() tea.Msg {
		result, err := m.svc.Switch(context.Background(), name)
		return opDoneMsg{seq: seq, kind: opSwitch, account: name, result: result, err: err}
	}
	return m, tea.Batch(func() tea.Msg { return m.spin.Tick() }, cmd)
}

// handleOpDone processes the result of a completed background operation.
func (m Model) handleOpDone(msg opDoneMsg) (Model, tea.Cmd) {
	// Drop stale results.
	if m.op == nil || msg.seq != m.op.seq {
		return m, nil
	}
	kind := m.op.kind
	loginAfter := m.op.loginAfter
	account := msg.account
	m.op = nil
	m.modal = nil

	if msg.err != nil {
		m.reload()
		return m, m.setStatus(fmt.Sprintf("%s failed: %s", kind, msg.err), levelError)
	}

	m.reload()

	switch kind {
	case opSwitch:
		res, _ := msg.result.(manager.SwitchResult)
		if m.isVisible(account) {
			m.selected = account
		}
		// Re-apply: if the active account is now something else, follow it.
		// An env-based switch leaves the activated account selected, which
		// is the one the feedback panel and its keys refer to.
		if active, ok := m.overview.Active(); ok && m.isVisible(active.Name) && res.ApplyHint == "" {
			m.selected = active.Name
		}
		if res.ApplyHint != "" {
			m.feedback = m.newFeedback(res)
		}
		if len(res.Warnings) > 0 {
			msgs := make([]string, 0, len(res.Warnings))
			for _, w := range res.Warnings {
				msgs = append(msgs, warningMessage(w))
			}
			return m, m.setStatus("switched (warnings: "+strings.Join(msgs, "; ")+")", levelWarning)
		}
		if m.feedback != nil {
			if m.panelFits() {
				return m, nil
			}
			return m, m.setStatus(feedbackStatus(m.feedback), levelInfo)
		}
		return m, m.setStatus("switched to "+account, levelInfo)

	case opSave:
		res, _ := msg.result.(manager.SaveResult)
		return m, m.setStatus("credentials saved for "+res.Account, levelInfo)

	case opDelete:
		res, _ := msg.result.(manager.DeleteResult)
		if len(res.Warnings) > 0 {
			msgs := make([]string, 0, len(res.Warnings))
			for _, w := range res.Warnings {
				msgs = append(msgs, warningMessage(w))
			}
			return m, m.setStatus("deleted "+account+" (warnings: "+strings.Join(msgs, "; ")+")", levelWarning)
		}
		return m, m.setStatus("deleted "+account, levelInfo)

	case opCreate:
		res, _ := msg.result.(manager.CreateResult)
		if m.isVisible(res.Account) {
			m.selected = res.Account
		}
		cmd := m.setStatus("created "+res.Account, levelInfo)
		if !loginAfter {
			return m, cmd
		}
		loginM, loginCmd := m.startLogin(res.Account)
		m = loginM
		return m, tea.Batch(cmd, loginCmd)

	case opUpdate:
		if m.isVisible(account) {
			m.selected = account
		}
		return m, m.setStatus("updated "+account, levelInfo)
	}

	return m, nil
}

// updateModal routes any message to the open modal.
func (m Model) updateModal(msg tea.Msg) (Model, tea.Cmd) {
	if m.modal == nil {
		return m, nil
	}
	return m.modal.update(m, msg)
}

// viewModal draws the open modal centred over base.
func (m Model) viewModal(base string) string {
	if m.modal == nil {
		return base
	}
	return m.modal.view(m)
}

// openSave opens the save-ADC dialog for name.
func (m Model) openSave(name string) (Model, tea.Cmd) {
	acc, ok := m.overview.Account(name)
	if !ok {
		return m, nil
	}
	m.modal = newSaveDialog(acc, m.overview)
	return m, nil
}

// openDelete opens the delete confirmation dialog for name.
func (m Model) openDelete(name string) (Model, tea.Cmd) {
	acc, ok := m.overview.Account(name)
	if !ok {
		return m, nil
	}
	plan, err := m.svc.DeletePlan(name)
	if err != nil {
		return m, m.setStatus("cannot delete "+name+": "+err.Error(), levelError)
	}
	m.modal = newDeleteDialog(acc, m.providerInfo(acc.Provider), plan)
	return m, nil
}

// openCreate opens the create account form, preselecting the provider of the
// selected account.
func (m Model) openCreate() (Model, tea.Cmd) {
	preferred := provider.ID("")
	if acct, ok := m.selectedAccount(); ok {
		preferred = acct.Provider
	}
	m.modal = newCreateForm(m.overview, preferred)
	return m, nil
}

// openEdit opens the form prefilled for name; only editable fields change.
func (m Model) openEdit(name string) (Model, tea.Cmd) {
	acct, ok := m.overview.Account(name)
	if !ok {
		return m, nil
	}
	form, ok := newEditForm(m.overview, acct)
	if !ok {
		return m, m.setStatus("provider of "+name+" is not available in this build", levelError)
	}
	m.modal = form
	return m, nil
}

// openRun opens the run prompt (B3's newRunPrompt).
func (m Model) openRun(name string) (Model, tea.Cmd) {
	cli, bin := m.runCLI(name)
	m.modal = newRunPrompt(name, cli, bin)
	return m, nil
}

// startShell and startLogin forward to the exec flows.
func (m Model) startShell(name string, scope manager.ShellScope) (Model, tea.Cmd) {
	return m.beginShell(name, scope)
}
func (m Model) startLogin(name string) (Model, tea.Cmd) { return m.beginLogin(name) }

// ─── Save dialog ─────────────────────────────────────────────────────────────

type focusTarget int

const (
	focusCancel focusTarget = iota
	focusConfirm
)

type saveDialog struct {
	account  manager.AccountStatus
	overview manager.Overview
	focus    focusTarget
}

func newSaveDialog(acc manager.AccountStatus, overview manager.Overview) *saveDialog {
	return &saveDialog{account: acc, overview: overview, focus: focusCancel}
}

func (d *saveDialog) update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q":
		m.modal = nil
		return m, nil
	case "tab", "shift+tab", "left", "right", "h", "l":
		if d.focus == focusCancel {
			d.focus = focusConfirm
		} else {
			d.focus = focusCancel
		}
		m.modal = d
		return m, nil
	case "enter", " ":
		m.modal = nil
		if d.focus == focusCancel {
			return m, nil
		}
		if m.busy() {
			return m, m.setStatus("Busy: "+m.op.desc, levelWarning)
		}
		m.seq++
		seq := m.seq
		name := d.account.Name
		m.op = &operation{kind: opSave, account: name, desc: "saving credentials", seq: seq, started: m.now()}
		cmd := func() tea.Msg {
			result, err := m.svc.SaveCredentials(context.Background(), name)
			return opDoneMsg{seq: seq, kind: opSave, account: name, result: result, err: err}
		}
		return m, tea.Batch(func() tea.Msg { return m.spin.Tick() }, cmd)
	}
	return m, nil
}

func (d *saveDialog) view(m Model) string {
	var sb strings.Builder
	sb.WriteString("Save ADC for " + d.account.Name + "\n\n")
	if m.overview.DefaultADCExists {
		sb.WriteString("Default ADC: " + m.overview.DefaultADCPath + "\n")
		if !m.overview.DefaultADCModTime.IsZero() {
			sb.WriteString("Modified: " + m.overview.DefaultADCModTime.Format("2006-01-02 15:04:05") + "\n")
		}
		if d.account.Email != "" {
			sb.WriteString("Email: " + d.account.Email + "\n")
		}
	} else {
		sb.WriteString("No default ADC file found.\n")
	}
	sb.WriteString("\n")
	cancelLabel := "[ Cancel ]"
	confirmLabel := "[ Save ]"
	if d.focus == focusCancel {
		cancelLabel = "[*Cancel*]"
	} else {
		confirmLabel = "[*Save*]"
	}
	sb.WriteString(cancelLabel + "  " + confirmLabel)
	return sb.String()
}
