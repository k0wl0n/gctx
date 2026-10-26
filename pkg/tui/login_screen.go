package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

const (
	loginTickInterval = 200 * time.Millisecond
	maxLoginLines     = 500
	loginSplitWidth   = 100 // from this width the output pane sits beside the steps
)

type (
	loginEventMsg struct {
		id    int
		event provider.LoginEvent
	}
	loginFinishedMsg struct {
		id  int
		err error
	}
	loginTickMsg struct{ id int }
)

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepDone
	stepFailed
)

type loginStep struct {
	label  string
	detail string
	state  stepState
	since  time.Time
}

// loginSession is one streamed login: the provider's Login runs in a
// goroutine and reports through events. It is shared by pointer between
// model copies.
type loginSession struct {
	id      int
	account string
	svc     Service
	ctx     context.Context
	cancel  context.CancelFunc

	events    chan tea.Msg
	abandoned chan struct{} // closed when nobody listens any more
	start     sync.Once
	abandon   sync.Once

	steps     []loginStep
	url, code string
	lines     []string
	fullLog   bool
	frame     int

	stopping bool         // cancel requested; waiting for the child to exit
	leaving  loginOutcome // what to record once a stopped login ends
	abort    bool         // end the queue once a stopped login ends
	finished bool
	err      error
}

func tickLogin(id int) tea.Cmd {
	return tea.Tick(loginTickInterval, func(time.Time) tea.Msg { return loginTickMsg{id: id} })
}

func (m Model) tick(id int) tea.Cmd {
	if m.loginTick == nil {
		return nil
	}
	return m.loginTick(id)
}

// startStreamLogin shows the login screen and runs the account's login there.
func (m Model) startStreamLogin(name string) (Model, tea.Cmd) {
	m.loginSeq++
	ctx, cancel := context.WithCancel(context.Background())
	s := &loginSession{
		id: m.loginSeq, account: name, svc: m.svc, ctx: ctx, cancel: cancel,
		events: make(chan tea.Msg), abandoned: make(chan struct{}),
	}
	m.login = s
	m.screen = screenLogin
	if m.debugLog != nil {
		m.debugLog.Info("login", "account", name, "mode", "streamed")
	}
	return m, tea.Batch(s.next(), m.tick(s.id))
}

// next waits for the session's next message; the first call starts the login.
func (s *loginSession) next() tea.Cmd {
	return func() tea.Msg {
		s.start.Do(func() { go s.run() })
		select {
		case msg, ok := <-s.events:
			if !ok {
				return nil
			}
			return msg
		case <-s.abandoned:
			return nil
		}
	}
}

func (s *loginSession) run() {
	send := func(msg tea.Msg) {
		select {
		case s.events <- msg:
		case <-s.abandoned:
		}
	}
	_, err := s.svc.Login(s.ctx, s.account, manager.LoginIO{
		Progress: func(event provider.LoginEvent) { send(loginEventMsg{id: s.id, event: event}) },
	})
	send(loginFinishedMsg{id: s.id, err: err})
}

// closeLoginSession stops listening to the session and cancels it if still running.
func (m *Model) closeLoginSession() {
	if s := m.login; s != nil {
		s.cancel()
		s.abandon.Do(func() { close(s.abandoned) })
		m.login = nil
	}
}

func (s *loginSession) stepFor(label string) *loginStep {
	for i := range s.steps {
		if s.steps[i].label == label {
			return &s.steps[i]
		}
	}
	s.steps = append(s.steps, loginStep{label: label})
	return &s.steps[len(s.steps)-1]
}

func (s *loginSession) apply(event provider.LoginEvent, now time.Time) {
	switch event.Kind {
	case provider.LoginStepPending:
		s.stepFor(event.Step)
	case provider.LoginStepStarted:
		step := s.stepFor(event.Step)
		step.state, step.since = stepRunning, now
	case provider.LoginStepDone:
		step := s.stepFor(event.Step)
		step.state, step.detail = stepDone, cleanLine(event.Message)
	case provider.LoginStepFailed:
		step := s.stepFor(event.Step)
		step.state, step.detail = stepFailed, ""
	case provider.LoginDeviceCode:
		s.url, s.code = cleanLine(event.URL), cleanLine(event.Code)
	case provider.LoginOutput:
		s.lines = append(s.lines, cleanLine(event.Output))
		if len(s.lines) > maxLoginLines {
			s.lines = s.lines[len(s.lines)-maxLoginLines:]
		}
	}
}

// cleanLine drops escape sequences and control characters from CLI output so
// it cannot move the cursor or recolour the screen.
func cleanLine(line string) string {
	line = strings.ReplaceAll(ansi.Strip(line), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, line)
}

func (m Model) handleLoginEvent(msg loginEventMsg) (Model, tea.Cmd) {
	s := m.login
	if s == nil || s.id != msg.id {
		return m, nil
	}
	s.apply(msg.event, m.now())
	return m, s.next()
}

func (m Model) handleLoginTick(msg loginTickMsg) (Model, tea.Cmd) {
	s := m.login
	if s == nil || s.id != msg.id || s.finished {
		return m, nil
	}
	s.frame++
	return m, m.tick(s.id)
}

func (m Model) handleLoginFinished(msg loginFinishedMsg) (Model, tea.Cmd) {
	s := m.login
	if s == nil || s.id != msg.id {
		return m, nil
	}
	s.finished, s.err = true, msg.err
	switch {
	case msg.err == nil:
		if m.debugLog != nil {
			m.debugLog.Info("login", "account", s.account, "result", "ok")
		}
		return m.leaveLogin(loginSucceeded, false)
	case s.stopping:
		return m.leaveLogin(s.leaving, s.abort)
	}
	if m.debugLog != nil {
		m.debugLog.Info("login", "account", s.account, "result", "failed")
	}
	for i := range s.steps {
		if s.steps[i].state == stepRunning {
			s.steps[i].state = stepFailed
		}
	}
	return m, nil
}

// updateLogin handles keys on the login screen.
func (m Model) updateLogin(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	s := m.login
	if s == nil {
		m.screen = screenMain
		return m, nil
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		return m.stopLogin(loginSkipped, true)
	case "n":
		return m.stopLogin(loginSkipped, false)
	case "v":
		s.fullLog = !s.fullLog
	case "o":
		return m.openLoginURL()
	case "y":
		return m.copyLoginCode()
	}
	return m, nil
}

// stopLogin ends the login in progress. A running login is cancelled and
// left once its child has exited; a failed one is left at once.
func (m Model) stopLogin(outcome loginOutcome, abort bool) (Model, tea.Cmd) {
	s := m.login
	switch {
	case s.finished:
		return m.leaveLogin(loginFailed, abort)
	case !s.stopping:
		s.stopping, s.leaving, s.abort = true, outcome, abort
		s.cancel()
	}
	return m, nil
}

func (m Model) openLoginURL() (Model, tea.Cmd) {
	url := m.login.url
	switch {
	case url == "":
		return m, m.setStatus("no sign-in page yet", levelInfo)
	case !strings.HasPrefix(url, "https://"):
		return m, m.setStatus("refusing to open a page that is not https", levelWarning)
	case m.openURL == nil:
		return m, m.setStatus("cannot open a browser here; open the page by hand", levelInfo)
	}
	if err := m.openURL(url); err != nil {
		return m, m.setStatus("could not open the browser; open the page by hand", levelWarning)
	}
	return m, m.setStatus("opened browser", levelInfo)
}

func (m Model) copyLoginCode() (Model, tea.Cmd) {
	code := m.login.code
	if code == "" {
		return m, m.setStatus("no code yet", levelInfo)
	}
	return m, tea.Batch(tea.SetClipboard(code), m.setStatus("copied code", levelInfo))
}

// ─── View ────────────────────────────────────────────────────────────────────

// viewLogin draws the login screen: header, rule, body, rule, status line and
// hint line.
func (m Model) viewLogin() string {
	s := m.login
	if s == nil {
		return ""
	}
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	account, _ := m.overview.Account(s.account)
	info := m.providerInfo(account.Provider)
	cli := info.CLI.Name
	if cli == "" {
		cli = info.Label
	}
	bodyHeight := max(m.height-5, 1)
	out := []string{m.loginHeader(info, g, st), hRule(g, st, m.width)}
	out = append(out, fitLines(m.loginBody(s, info, cli, bodyHeight, g, st), bodyHeight, m.width)...)
	out = append(out, hRule(g, st, m.width), m.renderStatus(m.width, g, st), m.loginHints(s, cli, g, st))
	return strings.Join(out, "\n")
}

// loginHeader is "accounts › BADGE name › login" with the queue position on the right.
func (m Model) loginHeader(info manager.ProviderInfo, g glyphs, st styles) string {
	s := m.login
	left := st.Dim.Render("accounts "+g.Caret) + " " + renderBadge(info.Badge, m.profile, g) + " " + st.Bold.Render(s.account) + " " + st.Dim.Render(g.Caret+" login")
	q := m.queue
	if q == nil || len(q.entries) < 2 {
		return truncateCells(left, m.width, g.Ellipsis)
	}
	right := fmt.Sprintf("queue %d of %d", q.next+1, len(q.entries))
	if upcoming := q.upcoming(); len(upcoming) > 0 {
		right += " " + g.Dot + " next " + strings.Join(upcoming, ", ")
	}
	room := m.width - ansi.StringWidth(left) - 2
	if room < 12 {
		return truncateCells(left, m.width, g.Ellipsis)
	}
	right = truncateCells(right, room, g.Ellipsis)
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	return left + strings.Repeat(" ", gap) + st.Dim.Render(right)
}

func (m Model) loginHints(s *loginSession, cli string, g glyphs, st styles) string {
	var left string
	switch {
	case s.stopping:
		left = st.Warn.Render("stopping " + cli + g.Ellipsis)
	case s.finished:
		left = keyChip("esc", st) + " back to list"
		if q := m.queue; q != nil && len(q.upcoming()) > 0 {
			left += "  " + keyChip("n", st) + " next in queue"
		}
	default:
		left = keyChip("esc", st) + " cancel (stops " + cli + ", account unchanged)"
		if q := m.queue; q != nil && len(q.entries) > 1 {
			left += "  " + keyChip("n", st) + " skip to next in queue"
		}
	}
	right := st.Dim.Render("on success: status " + g.Arrow + " READY, back to list")
	if gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(right); gap >= 2 && !s.finished && !s.stopping {
		return left + strings.Repeat(" ", gap) + right
	}
	return truncateCells(left, m.width, g.Ellipsis)
}

func (m Model) loginBody(s *loginSession, info manager.ProviderInfo, cli string, height int, g glyphs, st styles) []string {
	if s.fullLog {
		return m.outputPane(s, cli, m.width, height, true, g, st)
	}
	if m.width < loginSplitWidth {
		left := m.loginProgress(s, info, cli, m.width, g, st)
		paneHeight := height - len(left) - 1
		if paneHeight < 2 {
			return left
		}
		return append(append(left, ""), m.outputPane(s, cli, m.width, paneHeight, false, g, st)...)
	}
	paneWidth := max(m.width*2/5, 40)
	leftWidth := m.width - paneWidth - 3
	left := m.loginProgress(s, info, cli, leftWidth, g, st)
	right := m.outputPane(s, cli, paneWidth, height, false, g, st)
	sep := st.Sep.Render(g.VSep)
	rows := make([]string, height)
	for i := range rows {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		rows[i] = padCells(l, leftWidth) + " " + sep + " " + r
	}
	return rows
}

// loginProgress is the steps, the sign-in box and any failure, width cells wide.
func (m Model) loginProgress(s *loginSession, info manager.ProviderInfo, cli string, width int, g glyphs, st styles) []string {
	var lines []string
	if len(s.steps) == 0 && !s.finished {
		lines = append(lines, st.Dim.Render(g.Spinner[s.frame%len(g.Spinner)]+" starting "+cli+g.Ellipsis))
	}
	for _, step := range s.steps {
		lines = append(lines, truncateCells(m.stepLine(step, s, g, st), width, g.Ellipsis))
	}
	if s.url != "" || s.code != "" {
		lines = append(lines, "")
		lines = append(lines, loginBox(s, info, m, width, g, st)...)
	}
	switch {
	case s.stopping:
		lines = append(lines, "", st.Warn.Render("stopping "+cli+g.Ellipsis+" the account stays unchanged"))
	case s.finished && s.err != nil:
		lines = append(lines, "")
		for _, row := range strings.Split(ansi.Wrap(g.No+" login failed: "+cleanLine(s.err.Error()), width, ""), "\n") {
			lines = append(lines, st.Error.Render(row))
		}
	}
	return lines
}

// keyChip is a key on a small dim background; without colour it is the bold key.
func keyChip(label string, st styles) string {
	if st.prof <= colorprofile.ASCII {
		return st.Bold.Render(label)
	}
	return st.Selected.Bold(true).Render(" " + label + " ")
}

func (m Model) stepLine(step loginStep, s *loginSession, g glyphs, st styles) string {
	switch step.state {
	case stepDone:
		line := st.OK.Render(g.Yes) + " " + st.Bold.Render(step.label)
		if step.detail != "" {
			line += " " + st.Dim.Render(step.detail)
		}
		return line
	case stepRunning:
		wait := "running"
		if s.code != "" {
			wait = "waiting"
		}
		elapsed := max(m.now().Sub(step.since), 0)
		return st.Warn.Render(g.Spinner[s.frame%len(g.Spinner)]) + " " + st.Bold.Render(step.label) +
			" " + st.Dim.Render(fmt.Sprintf("%s %d:%02d", wait, int(elapsed.Minutes()), int(elapsed.Seconds())%60))
	case stepFailed:
		return st.Error.Render(g.No) + " " + st.Error.Render(step.label)
	}
	return st.Dim.Render(g.Missing + " " + step.label)
}

// loginBox is the bordered box with the sign-in page and the code in large,
// spaced characters.
func loginBox(s *loginSession, info manager.ProviderInfo, m Model, width int, g glyphs, st styles) []string {
	boxWidth := width
	if boxWidth < 12 {
		return nil
	}
	inner := boxWidth - 4
	border := func(text string) string { return tint(info.Badge.Color, m.profile, text) }
	row := func(content string) string {
		content = truncateCells(content, inner, g.Ellipsis)
		return border(g.Box[5]) + " " + padCells(content, inner) + " " + border(g.Box[5])
	}
	rule := strings.Repeat(g.Box[4], boxWidth-2)

	lines := []string{border(g.Box[0] + rule + g.Box[1])}
	if s.url != "" {
		lines = append(lines, row(st.Dim.Render("open this page in any browser")), row(st.Bold.Render(s.url)))
	}
	if s.code != "" {
		code := spaced(s.code)
		if runeLen(code) > inner {
			code = s.code
		}
		lines = append(lines, row(st.Dim.Render("and enter the code")), row(st.Bold.Render(code)))
	}
	keysRow := keyChip("o", st) + " open browser   " + keyChip("y", st) + " copy code"
	lines = append(lines, row(keysRow), border(g.Box[2]+rule+g.Box[3]))
	return lines
}

// spaced puts a space between every character: "F7K2" becomes "F 7 K 2".
func spaced(code string) string {
	runes := []rune(code)
	parts := make([]string, len(runes))
	for i, r := range runes {
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

// outputPane is the CLI output: a title row, the newest lines that fit
// (wrapped) and, at the bottom, the note that tokens stay with the CLI.
func (m Model) outputPane(s *loginSession, cli string, width, height int, full bool, g glyphs, st styles) []string {
	title, hint := cli+" output", "v full log"
	if full {
		title, hint = cli+" output "+g.Dot+" full log", "v back"
	}
	gap := max(width-runeLen(title)-runeLen(hint), 1)
	lines := []string{truncateCells(st.Dim.Render(title)+strings.Repeat(" ", gap)+st.Dim.Render(hint), width, g.Ellipsis)}

	var footer []string
	note := "tokens stay in the isolated " + cli + " dir; " + config.AppName + " never reads them"
	if height >= 8 {
		for _, row := range strings.Split(ansi.Wrap(note, width, ""), "\n") {
			footer = append(footer, st.Dim.Render(row))
		}
	}
	room := height - 1 - len(footer)
	if len(footer) > 0 {
		room-- // blank line above the note
	}
	var wrapped []string
	for _, line := range s.lines {
		wrapped = append(wrapped, strings.Split(ansi.Wrap(line, width, ""), "\n")...)
	}
	if room < len(wrapped) {
		wrapped = wrapped[len(wrapped)-max(room, 0):]
	}
	lines = append(lines, wrapped...)
	if len(footer) == 0 {
		return lines
	}
	for len(lines) < height-len(footer) {
		lines = append(lines, "")
	}
	return append(lines, footer...)
}

// fitLines pads or cuts lines to exactly height rows of at most width cells.
func fitLines(lines []string, height, width int) []string {
	out := make([]string, height)
	for i := range out {
		if i < len(lines) {
			out[i] = truncateCells(lines[i], width, "")
		}
	}
	return out
}
