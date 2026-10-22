package tui

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
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
	screenLogin
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
	opUpdate opKind = "update"
)

// operation is the single in-flight mutating operation.
type operation struct {
	kind    opKind
	account string
	desc    string // lower-case description used in "Busy: <desc>"
	seq     int
	started time.Time
	// loginAfter runs the login flow once a create succeeds.
	loginAfter bool
}

type credPhase int

const (
	credIdle credPhase = iota
	credChecking
	credDone
	credFailed
)

type credState struct {
	phase    credPhase
	seq      int
	err      error
	progress map[provider.ID]checkProgress // one entry per provider being checked
	finished time.Time                     // when the last check completed
}

// checkProgress is one provider's part of a credential check.
type checkProgress struct {
	done    bool
	total   int // accounts of the provider
	checked int // accounts with a verdict
	warning string
	failed  bool // the check itself returned an error
	timeout time.Duration
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
		seq      int
		provider provider.ID // "" for a check of every provider
		report   manager.CredentialReport
		err      error
	}
	envLoadedMsg struct{ envs map[string][]string }
	opDoneMsg    struct {
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
	visible  []int               // indices into overview.Accounts after filtering, in display order
	groups   []listGroup         // visible rows by provider, in display order
	nav      []navRow            // selectable rows: visible accounts plus folded-group placeholders
	selected string              // selected account name
	offset   int                 // first visible row
	envs     map[string][]string // Env per account, loaded asynchronously

	tab         int             // 0 is All, then one tab per listed provider
	folded      map[string]bool // provider id -> group folded on the All tab
	paneOpen    bool            // detail pane, drawn at tierWide only
	needsAction bool            // only rows that need action
	sort        sortKey

	screen   screenKind
	modal    modal
	feedback *feedbackPanel // after activating an env-based account
	filter   filterState

	op        *operation
	creds     credState
	seq       int
	quitArmed time.Time
	spin      spinner.Model

	status        statusMsg
	statusSeq     int
	width, height int
	profile       colorprofile.Profile // from tea.ColorProfileMsg; TrueColor until then
	history       []string             // run prompt history, memory only

	login     *loginSession // the streamed login on screenLogin
	queue     *loginQueue   // logins in progress, one entry for a single login
	loginSeq  int
	openURL   func(string) error   // opens the browser; replaced in tests
	loginTick func(id int) tea.Cmd // animates the login screen; replaced in tests
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
		spin: spinner.New(spinner.WithSpinner(spinner.Spinner{
			Frames: newGlyphs(asciiRequested()).Spinner, FPS: time.Second / 10,
		})),
		openURL:   openBrowser,
		loginTick: tickLogin,
		filter:    filterState{input: newLineInput("/")},
		profile:   colorprofile.TrueColor,
		folded:    map[string]bool{},
		paneOpen:  true,
	}
	m.reload()
	return m
}

// Init starts the credential check and loads the env lines after the first frame.
func (m Model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return startCredsMsg{} }, m.loadEnvCmd())
}

// loadEnvCmd reads every account's env lines for the header and the detail
// pane. Env is local only; an account whose env fails is left out.
func (m Model) loadEnvCmd() tea.Cmd {
	svc := m.svc
	var names []string
	for _, account := range m.overview.Accounts {
		names = append(names, account.Name)
	}
	if len(names) == 0 {
		return nil
	}
	return func() tea.Msg {
		envs := map[string][]string{}
		for _, name := range names {
			if env, err := svc.Env(name); err == nil {
				envs[name] = env
			}
		}
		return envLoadedMsg{envs: envs}
	}
}

func (m Model) spinnerFrame() string { return m.spin.View() }

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
		m.visible, m.groups, m.nav, m.selected, m.offset = nil, nil, nil, "", 0
		return
	}
	m.overview = overview
	if m.tab >= m.tabCount() {
		m.tab = 0
	}
	m.rebuildVisible()

	switch {
	case len(m.nav) == 0:
		m.selected = ""
	case previous != "" && m.isVisible(previous):
		m.selected = previous
	case previous != "":
		m.selected = m.rowKey(m.nav[min(previousIndex, len(m.nav)-1)])
	default:
		m.selected = m.rowKey(m.nav[0])
		if active, ok := m.overview.Active(); ok && m.isVisible(active.Name) {
			m.selected = active.Name
		}
	}
}

// navRow is one selectable row: an account, or (account < 0) the
// "… N more ready" placeholder that a folded group ends with.
type navRow struct {
	account int
	group   provider.ID
}

// foldKey is the selection key of a folded group's placeholder. Account
// names cannot contain NUL, so it never matches an account.
func foldKey(id provider.ID) string { return "\x00fold:" + string(id) }

func (m Model) rowKey(r navRow) string {
	if r.account < 0 {
		return foldKey(r.group)
	}
	return m.overview.Accounts[r.account].Name
}

// selectedPlaceholder returns the folded group whose placeholder is selected.
func (m Model) selectedPlaceholder() (listGroup, bool) {
	for _, g := range m.groups {
		if g.hidden > 0 && m.selected == foldKey(g.info.ID) {
			return g, true
		}
	}
	return listGroup{}, false
}

// rebuildVisible applies the tab, filters, sort and folds to the overview.
func (m *Model) rebuildVisible() {
	m.groups = m.buildGroups()
	m.visible, m.nav = nil, nil
	for _, g := range m.groups {
		m.visible = append(m.visible, g.rows...)
		for _, idx := range g.rows {
			m.nav = append(m.nav, navRow{account: idx, group: g.info.ID})
		}
		if g.hidden > 0 {
			m.nav = append(m.nav, navRow{account: -1, group: g.info.ID})
		}
	}
}

// refilter rebuilds the rows after a view change and keeps the selection
// when it is still shown, else selects the row nearest its old position.
func (m *Model) refilter() {
	previousIndex := m.selectedIndex()
	m.rebuildVisible()
	switch {
	case len(m.nav) == 0:
		m.selected = ""
	case m.selected != "" && m.isVisible(m.selected):
	default:
		m.selected = m.rowKey(m.nav[min(previousIndex, len(m.nav)-1)])
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
	for row, r := range m.nav {
		if m.rowKey(r) == name {
			return row
		}
	}
	return -1
}

func (m Model) selectedIndex() int { return max(m.visibleIndex(m.selected), 0) }

// startCredsCheck begins the asynchronous credential check: one command
// per provider with accounts, each reporting as it finishes.
func (m *Model) startCredsCheck() tea.Cmd {
	m.seq++
	m.creds = credState{phase: credChecking, seq: m.seq, progress: map[provider.ID]checkProgress{}, finished: m.creds.finished}
	var cmds []tea.Cmd
	for _, info := range m.listedProviders() {
		m.creds.progress[info.ID] = checkProgress{total: info.Count, timeout: info.CheckTimeout}
		cmds = append(cmds, checkProviderCmd(m.svc, m.seq, info.ID))
	}
	if len(cmds) == 0 {
		if len(m.overview.Accounts) == 0 {
			m.creds.phase = credDone
			return nil
		}
		// No provider metadata: check everything at once.
		cmds = append(cmds, checkCredsCmd(m.svc, m.seq))
	}
	if !m.busy() {
		spin := m.spin
		cmds = append(cmds, func() tea.Msg { return spin.Tick() })
	}
	return tea.Batch(cmds...)
}

func checkProviderCmd(svc Service, seq int, id provider.ID) tea.Cmd {
	return func() tea.Msg {
		report, err := svc.CheckProviderCredentials(context.Background(), id)
		return credsCheckedMsg{seq: seq, provider: id, report: report, err: err}
	}
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
	case tea.ColorProfileMsg:
		m.profile = msg.Profile
		return m, nil
	case startCredsMsg:
		return m, m.startCredsCheck()
	case tea.FocusMsg:
		m.reload()
		return m, m.loadEnvCmd()
	case statusExpiredMsg:
		if m.status.id == msg.id && m.status.level == levelInfo {
			m.clearStatus()
		}
		return m, nil
	case spinner.TickMsg:
		if !m.busy() && m.creds.phase != credChecking {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case credsCheckedMsg:
		return m.handleCreds(msg)
	case envLoadedMsg:
		m.envs = msg.envs
		return m, nil
	case opDoneMsg:
		next, cmd := m.handleOpDone(msg) // create and delete change the accounts
		return next, tea.Batch(cmd, next.loadEnvCmd())
	case execDoneMsg:
		return m.handleExecDone(msg)
	case loginEventMsg:
		return m.handleLoginEvent(msg)
	case loginFinishedMsg:
		return m.handleLoginFinished(msg)
	case loginTickMsg:
		return m.handleLoginTick(msg)
	}

	if key, ok := msg.(tea.KeyPressMsg); ok && m.tooSmall() {
		if m.screen == screenLogin {
			return m.updateLogin(key)
		}
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
	case m.screen == screenLogin:
		return m.updateLogin(key)
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
