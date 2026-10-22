package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/manager"
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
		return m, m.setStatus("deleted "+account, levelInfo)

	case opCreate:
		res, _ := msg.result.(manager.CreateResult)
		if m.isVisible(res.Account) {
			m.selected = res.Account
		}
		cmd := m.setStatus("created "+res.Account, levelInfo)
		loginM, loginCmd := m.startLogin(res.Account)
		m = loginM
		return m, tea.Batch(cmd, loginCmd)
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
	m.modal = newDeleteDialog(acc, m.overview)
	return m, nil
}

// openCreate opens the create account form.
func (m Model) openCreate() (Model, tea.Cmd) {
	m.modal = newCreateForm()
	return m, nil
}

// openRun opens the run prompt (B3's newRunPrompt).
func (m Model) openRun(name string) (Model, tea.Cmd) {
	m.modal = newRunPrompt(name)
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

// ─── Delete dialog ────────────────────────────────────────────────────────────

type deleteDialog struct {
	account      manager.AccountStatus
	overview     manager.Overview
	focus        focusTarget
	deleteGcloud bool // checkbox: also delete gcloud config
	gcloudActive bool // whether this is gcloud's active config (checkbox disabled)
}

func newDeleteDialog(acc manager.AccountStatus, overview manager.Overview) *deleteDialog {
	gcloudActive := overview.GcloudActiveConfig == acc.ConfigName
	return &deleteDialog{
		account:      acc,
		overview:     overview,
		focus:        focusCancel,
		deleteGcloud: !gcloudActive,
		gcloudActive: gcloudActive,
	}
}

func (d *deleteDialog) update(m Model, msg tea.Msg) (Model, tea.Cmd) {
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
	case "space", "x":
		if !d.gcloudActive {
			d.deleteGcloud = !d.deleteGcloud
		}
		m.modal = d
		return m, nil
	case "enter":
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
		delGcloud := d.deleteGcloud
		m.op = &operation{kind: opDelete, account: name, desc: "deleting", seq: seq, started: m.now()}
		cmd := func() tea.Msg {
			result, err := m.svc.Delete(context.Background(), name, manager.DeleteOptions{GcloudConfig: delGcloud})
			return opDoneMsg{seq: seq, kind: opDelete, account: name, result: result, err: err}
		}
		return m, tea.Batch(func() tea.Msg { return m.spin.Tick() }, cmd)
	}
	return m, nil
}

func (d *deleteDialog) view(m Model) string {
	var sb strings.Builder
	sb.WriteString("Delete account: " + d.account.Name + "\n\n")
	if d.account.ADCExists {
		sb.WriteString("ADC credentials will be removed.\n")
	}
	checkbox := "[ ] also delete gcloud config"
	if d.deleteGcloud {
		checkbox = "[x] also delete gcloud config"
	}
	if d.gcloudActive {
		checkbox += " (disabled: gcloud's active config)"
	}
	sb.WriteString(checkbox + "\n\n")
	cancelLabel := "[ Cancel ]"
	confirmLabel := "[ Delete ]"
	if d.focus == focusCancel {
		cancelLabel = "[*Cancel*]"
	} else {
		confirmLabel = "[*Delete*]"
	}
	sb.WriteString(cancelLabel + "  " + confirmLabel)
	return sb.String()
}

// ─── Create form ──────────────────────────────────────────────────────────────

type createField int

const (
	fieldName createField = iota
	fieldProject
	fieldSubmit
)

type createForm struct {
	nameInput     lineInput
	projectInput  lineInput
	focus         createField
	nameErr       string
	projectErr    string
	reuseWarning  string
	reuseAccepted bool
}

func newCreateForm() *createForm {
	name := newLineInput("Name: ")
	project := newLineInput("Project: ")
	return &createForm{nameInput: name, projectInput: project, focus: fieldName}
}

func (f *createForm) update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "esc":
		m.modal = nil
		return m, nil

	case "tab":
		switch f.focus {
		case fieldName:
			f.focus = fieldProject
		case fieldProject:
			f.focus = fieldSubmit
		case fieldSubmit:
			f.focus = fieldName
		}
		m.modal = f
		return m, nil

	case "shift+tab":
		switch f.focus {
		case fieldSubmit:
			f.focus = fieldProject
		case fieldProject:
			f.focus = fieldName
		case fieldName:
			f.focus = fieldSubmit
		}
		m.modal = f
		return m, nil

	case "enter":
		if f.focus == fieldName {
			f.focus = fieldProject
			m.modal = f
			return m, nil
		}
		if f.focus == fieldProject {
			f.focus = fieldSubmit
			m.modal = f
			return m, nil
		}
		// Submit.
		nameVal := strings.TrimSpace(f.nameInput.Value())
		projectVal := strings.TrimSpace(f.projectInput.Value())

		if err := manager.ValidateAccountName(nameVal); err != nil {
			f.nameErr = err.Error()
			f.focus = fieldName
			m.modal = f
			return m, nil
		}
		f.nameErr = ""
		if projectVal != "" {
			if err := manager.ValidateProjectID(projectVal); err != nil {
				f.projectErr = err.Error()
				f.focus = fieldProject
				m.modal = f
				return m, nil
			}
		}
		f.projectErr = ""

		if _, exists := m.overview.Account(nameVal); exists {
			if !f.reuseAccepted {
				f.reuseWarning = "Account '" + nameVal + "' already exists. Press Enter again to proceed anyway."
				f.reuseAccepted = true
				m.modal = f
				return m, nil
			}
		}
		f.reuseWarning = ""

		m.modal = nil
		if m.busy() {
			return m, m.setStatus("Busy: "+m.op.desc, levelWarning)
		}
		m.seq++
		seq := m.seq
		name := nameVal
		project := projectVal
		m.op = &operation{kind: opCreate, account: name, desc: "creating", seq: seq, started: m.now()}
		cmd := func() tea.Msg {
			result, err := m.svc.Create(context.Background(), name, project)
			return opDoneMsg{seq: seq, kind: opCreate, account: name, result: result, err: err}
		}
		return m, tea.Batch(func() tea.Msg { return m.spin.Tick() }, cmd)
	}

	// Forward typing to the active field.
	switch f.focus {
	case fieldName:
		f.nameInput.update(key)
	case fieldProject:
		f.projectInput.update(key)
	}
	m.modal = f
	return m, nil
}

func (f *createForm) view(m Model) string {
	var sb strings.Builder
	sb.WriteString("Create account\n\n")
	sb.WriteString(f.nameInput.View(f.focus == fieldName) + "\n")
	if f.nameErr != "" {
		sb.WriteString("  ! " + f.nameErr + "\n")
	}
	sb.WriteString(f.projectInput.View(f.focus == fieldProject) + "\n")
	if f.projectErr != "" {
		sb.WriteString("  ! " + f.projectErr + "\n")
	}
	if f.reuseWarning != "" {
		sb.WriteString("\n" + f.reuseWarning + "\n")
	}
	submitLabel := "[ Create ]"
	if f.focus == fieldSubmit {
		submitLabel = "[*Create*]"
	}
	sb.WriteString("\n" + submitLabel)
	return sb.String()
}
