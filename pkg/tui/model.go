package tui

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/manager"
)

const (
	minWidth, minHeight = 60, 12
	infoMessageTTL      = 5 * time.Second
	quitConfirmWindow   = 3 * time.Second
	historyLimit        = 50
)

type screenKind int

const (
	screenMain screenKind = iota
	screenHelp
	screenDetails
)

type statusLevel int

const (
	levelInfo statusLevel = iota
	levelWarning
	levelError
)

type statusMsg struct {
	id      int
	text    string
	level   statusLevel
	expires time.Time // zero: persists
}

type opKind string

const (
	opSwitch opKind = "switch"
	opSave   opKind = "save"
	opDelete opKind = "delete"
	opCreate opKind = "create"
)

// operation is the single in-flight mutating operation.
type operation struct {
	kind    opKind
	account string
	desc    string // lower-case description used in "Busy: <desc>"
	seq     int
	started time.Time
}

type credPhase int

const (
	credIdle credPhase = iota
	credChecking
	credDone
	credFailed
)

type credState struct {
	phase credPhase
	seq   int
	err   error
}

type filterState struct {
	editing bool
	input   lineInput
	query   string
}

// modal is a dialog drawn over the main view. B2 and B3 add implementations.
type modal interface {
	update(m Model, msg tea.Msg) (Model, tea.Cmd)
	view(m Model) string
}

// Asynchronous messages.
type (
	startCredsMsg   struct{}
	credsCheckedMsg struct {
		seq    int
		report manager.CredentialReport
		err    error
	}
	opDoneMsg struct {
		seq     int
		kind    opKind
		account string
		result  any // manager.SwitchResult, SaveResult, DeleteResult or CreateResult
		err     error
	}
	execDoneMsg struct {
		kind    execKind
		account string
		result  any // manager.LoginResult for login
		err     error
		exit    int // child exit code for run and shell
	}
	statusExpiredMsg struct{ id int }
)

// Model is the single Bubble Tea model.
type Model struct {
	svc      Service
	exec     func(tea.ExecCommand, tea.ExecCallback) tea.Cmd // tea.Exec; replaced in tests
	now      func() time.Time
	version  string
	debug    bool
	debugLog *slog.Logger
	goos     string

	overview manager.Overview
	loadErr  error
	visible  []int  // indices into overview.Accounts after filtering
	selected string // selected account name
	offset   int    // first visible row

	screen screenKind
	modal  modal
	filter filterState

	op        *operation
	creds     credState
	seq       int
	quitArmed time.Time
	spin      spinner.Model

	status        statusMsg
	statusSeq     int
	width, height int
	history       []string // run prompt history, memory only
}

// New builds the model and loads the local overview synchronously: it is a few
// file reads and never starts a subprocess.
func New(svc Service, opts Options) Model {
	return NewWithGOOS(svc, opts, "")
}

// NewWithGOOS is like New but lets callers inject the runtime OS string.
// An empty goos defaults to runtime.GOOS via the tokenize call site.
func NewWithGOOS(svc Service, opts Options, goos string) Model {
	m := Model{
		svc:      svc,
		exec:     tea.Exec,
		now:      time.Now,
		version:  opts.Version,
		debug:    opts.Debug,
		debugLog: slog.New(slog.NewTextHandler(io.Discard, nil)), // replaced in run.go
		goos:     goos,
		spin:     spinner.New(spinner.WithSpinner(spinner.Dot)),
		filter:   filterState{input: newLineInput("/")},
	}
	m.reload()
	return m
}

// Init starts the credential check after the first frame.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return startCredsMsg{} }
}

func (m Model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

func (m Model) busy() bool { return m.op != nil }

// selectedAccount returns the selected account if it is visible.
func (m Model) selectedAccount() (manager.AccountStatus, bool) {
	if m.selected == "" {
		return manager.AccountStatus{}, false
	}
	return m.overview.Account(m.selected)
}

// setStatus replaces the status line. Info messages expire; warnings and
// errors persist until the next action.
func (m *Model) setStatus(text string, level statusLevel) tea.Cmd {
	m.statusSeq++
	m.status = statusMsg{id: m.statusSeq, text: text, level: level}
	if text == "" || level != levelInfo {
		return nil
	}
	m.status.expires = m.now().Add(infoMessageTTL)
	id := m.status.id
	return tea.Tick(infoMessageTTL, func(time.Time) tea.Msg { return statusExpiredMsg{id: id} })
}

func (m *Model) clearStatus() { m.status = statusMsg{} }

// reload re-reads the local overview and rebuilds the visible rows, keeping the
// selection by name. When the selected account disappeared it moves to the
// nearest remaining row.
func (m *Model) reload() {
	previous := m.selected
	previousIndex := m.selectedIndex()
	overview, err := m.svc.Overview()
	m.loadErr = err
	if err != nil {
		m.overview = manager.Overview{}
		m.visible, m.selected, m.offset = nil, "", 0
		return
	}
	m.overview = overview
	m.rebuildVisible()

	switch {
	case len(m.visible) == 0:
		m.selected = ""
	case previous != "" && m.isVisible(previous):
		m.selected = previous
	case previous != "":
		m.selected = m.overview.Accounts[m.visible[min(previousIndex, len(m.visible)-1)]].Name
	default:
		m.selected = m.overview.Accounts[m.visible[0]].Name
		if active, ok := m.overview.Active(); ok && m.isVisible(active.Name) {
			m.selected = active.Name
		}
	}
}

// rebuildVisible applies the filter query to the overview.
func (m *Model) rebuildVisible() {
	query := strings.ToLower(m.filter.query)
	m.visible = nil
	for i, account := range m.overview.Accounts {
		if query == "" || matchesQuery(account, query) {
			m.visible = append(m.visible, i)
		}
	}
}

func matchesQuery(account manager.AccountStatus, lowerQuery string) bool {
	for _, field := range []string{account.Name, account.ProjectID, account.Email, account.ConfigName} {
		if strings.Contains(strings.ToLower(field), lowerQuery) {
			return true
		}
	}
	return false
}

func (m Model) isVisible(name string) bool { return m.visibleIndex(name) >= 0 }

// visibleIndex returns the row of name among the visible rows, or -1.
func (m Model) visibleIndex(name string) int {
	for row, i := range m.visible {
		if m.overview.Accounts[i].Name == name {
			return row
		}
	}
	return -1
}

func (m Model) selectedIndex() int { return max(m.visibleIndex(m.selected), 0) }

// startCredsCheck begins the asynchronous credential check.
func (m *Model) startCredsCheck() tea.Cmd {
	m.seq++
	m.creds = credState{phase: credChecking, seq: m.seq}
	return checkCredsCmd(m.svc, m.seq)
}

func checkCredsCmd(svc Service, seq int) tea.Cmd {
	return func() tea.Msg {
		report, err := svc.CheckCredentials(context.Background())
		return credsCheckedMsg{seq: seq, report: report, err: err}
	}
}

// Update only dispatches; behavior lives in the files that own it.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.dispatch(msg)
	return next, cmd
}

func (m Model) dispatch(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case startCredsMsg:
		return m, m.startCredsCheck()
	case tea.FocusMsg:
		m.reload()
		return m, nil
	case statusExpiredMsg:
		if m.status.id == msg.id && m.status.level == levelInfo {
			m.clearStatus()
		}
		return m, nil
	case spinner.TickMsg:
		if !m.busy() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case credsCheckedMsg:
		return m.handleCreds(msg)
	case opDoneMsg:
		return m.handleOpDone(msg)
	case execDoneMsg:
		return m.handleExecDone(msg)
	}

	if key, ok := msg.(tea.KeyPressMsg); ok && m.tooSmall() {
		if k := key.String(); k == "q" || k == "ctrl+c" {
			return m.updateMain(key)
		}
		return m, nil
	}
	if m.modal != nil {
		return m.updateModal(msg)
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch {
	case m.filter.editing:
		return m.updateFilter(key)
	case m.screen != screenMain:
		return m.updateOverlay(key)
	}
	return m.updateMain(key)
}

// View draws the current state.
func (m Model) View() tea.View {
	content := m.viewMain()
	if m.screen != screenMain && !m.tooSmall() {
		content = m.viewOverlay()
	}
	if m.modal != nil && !m.tooSmall() {
		content = m.viewModal(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.ReportFocus = true
	return view
}
