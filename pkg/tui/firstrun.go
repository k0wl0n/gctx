package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
)

type (
	discoveredMsg struct {
		result manager.DiscoveryResult
		err    error
	}
	importedMsg struct {
		result manager.ImportResult
		err    error
	}
)

// firstRunSource is a discovered CLI setup the user can tick.
type firstRunSource struct {
	info    manager.ProviderInfo
	source  manager.DiscoverySource
	checked bool
}

type reviewItem struct {
	info    manager.ProviderInfo
	item    manager.DiscoveredAccount
	checked bool
}

// firstRunState is the import offer shown on a machine without accounts.
type firstRunState struct {
	sources   []firstRunSource
	missing   []manager.ProviderInfo // providers whose CLI is not installed: shown dim
	items     []manager.DiscoveredAccount
	review    bool
	reviewed  []reviewItem
	focus     int
	importing bool
}

// discoverCmd looks for existing CLI setups when the machine has no accounts.
// Discovery reads files only.
func (m Model) discoverCmd() tea.Cmd {
	if len(m.overview.Accounts) > 0 || m.loadErr != nil || m.firstRunDone {
		return nil
	}
	svc := m.svc
	return func() tea.Msg {
		result, err := svc.Discover()
		return discoveredMsg{result: result, err: err}
	}
}

func (m Model) handleDiscovered(msg discoveredMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.debugLog.Warn("discovery failed", "error", msg.err)
		return m, nil
	}
	if len(m.overview.Accounts) > 0 || m.firstRunDone {
		return m, nil
	}
	m.firstRun = newFirstRun(m.overview, msg.result)
	return m, nil
}

// newFirstRun is nil when there is nothing a user could import.
func newFirstRun(overview manager.Overview, result manager.DiscoveryResult) *firstRunState {
	f := &firstRunState{items: result.Items}
	for _, info := range overview.Providers {
		switch {
		case !info.CLI.Found:
			f.missing = append(f.missing, info)
		default:
			for _, source := range result.Sources {
				if source.Provider == info.ID && source.Count > 0 {
					f.sources = append(f.sources, firstRunSource{info: info, source: source, checked: true})
				}
			}
		}
	}
	if len(f.sources) == 0 {
		return nil
	}
	f.focus = f.importButton()
	return f
}

// Focus order: the checkboxes, then the buttons.
func (f *firstRunState) checkboxCount() int {
	if f.review {
		return len(f.reviewed)
	}
	return len(f.sources)
}

func (f *firstRunState) importButton() int { return f.checkboxCount() }
func (f *firstRunState) secondButton() int { return f.checkboxCount() + 1 } // Review each, or Back
func (f *firstRunState) emptyButton() int  { return f.checkboxCount() + 2 }

func (f *firstRunState) focusCount() int {
	if f.review {
		return f.checkboxCount() + 2
	}
	return f.checkboxCount() + 3
}

// selected are the items that Import would create.
func (f *firstRunState) selected() []manager.DiscoveredAccount {
	var out []manager.DiscoveredAccount
	if f.review {
		for _, r := range f.reviewed {
			if r.checked {
				out = append(out, r.item)
			}
		}
		return out
	}
	for _, source := range f.sources {
		if !source.checked {
			continue
		}
		for _, item := range f.items {
			if item.Provider == source.info.ID {
				out = append(out, item)
			}
		}
	}
	return out
}

func (f *firstRunState) startReview() {
	f.reviewed = nil
	for _, source := range f.sources {
		for _, item := range f.items {
			if item.Provider == source.info.ID {
				f.reviewed = append(f.reviewed, reviewItem{info: source.info, item: item, checked: source.checked})
			}
		}
	}
	f.review, f.focus = true, 0
}

func (f *firstRunState) toggle() {
	switch {
	case f.focus >= f.checkboxCount():
	case f.review:
		f.reviewed[f.focus].checked = !f.reviewed[f.focus].checked
	default:
		f.sources[f.focus].checked = !f.sources[f.focus].checked
	}
}

func (m Model) updateFirstRun(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	f := m.firstRun
	key := msg.String()
	if key == "ctrl+c" || (key == "q" && !f.importing) {
		return m, tea.Quit
	}
	if f.importing {
		return m, nil
	}
	// The state is shared by pointer between model copies: work on a copy.
	next := *f
	f = &next
	m.firstRun = f
	switch key {
	case "?":
		return m.openHelp()
	case "c":
		m.firstRun, m.firstRunDone = nil, true
		return m.openCreate()
	case "down", "j", "tab":
		f.focus = (f.focus + 1) % f.focusCount()
	case "up", "k", "shift+tab":
		f.focus = (f.focus + f.focusCount() - 1) % f.focusCount()
	case "space":
		f.reviewed = append([]reviewItem(nil), f.reviewed...)
		f.sources = append([]firstRunSource(nil), f.sources...)
		f.toggle()
	case "esc":
		if f.review {
			f.review, f.focus = false, f.importButton()
		}
	case "enter":
		return m.activateFirstRun()
	}
	return m, nil
}

func (m Model) activateFirstRun() (Model, tea.Cmd) {
	f := m.firstRun
	switch {
	case f.focus < f.checkboxCount():
		f.reviewed = append([]reviewItem(nil), f.reviewed...)
		f.sources = append([]firstRunSource(nil), f.sources...)
		f.toggle()
	case f.focus == f.importButton():
		items := f.selected()
		if len(items) == 0 {
			return m, m.setStatus("Nothing is selected to import", levelInfo)
		}
		f.importing = true
		svc := m.svc
		return m, func() tea.Msg {
			result, err := svc.ImportAccounts(items)
			return importedMsg{result: result, err: err}
		}
	case f.focus == f.secondButton() && f.review:
		f.review, f.focus = false, f.importButton()
	case f.focus == f.secondButton():
		f.startReview()
	case f.focus == f.emptyButton() && !f.review:
		m.firstRun, m.firstRunDone = nil, true
	}
	return m, nil
}

func (m Model) handleImported(msg importedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if m.firstRun != nil {
			m.firstRun.importing = false
		}
		return m, m.setStatus("Import failed: "+msg.err.Error(), levelError)
	}
	m.firstRun, m.firstRunDone = nil, true
	m.tab = 0
	m.reload()
	level := levelInfo
	text := fmt.Sprintf("imported %d %s", len(msg.result.Imported), plural(len(msg.result.Imported), "account"))
	if skipped := msg.result.Skipped; len(skipped) > 0 {
		level = levelWarning
		text += fmt.Sprintf(" · skipped %d: %s (%s)", len(skipped), skipped[0].Name, skipped[0].Reason)
	}
	status := m.setStatus(text, level)
	return m, tea.Batch(status, m.startCredsCheck(), m.loadEnvCmd())
}

func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// viewFirstRun draws the import offer as a bordered panel filling the screen,
// the hint line on its last row.
func (m Model) viewFirstRun() string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	f := m.firstRun

	title := config.AppName + " · no accounts yet"
	intro := []string{"found existing CLI setups you can import.", "nothing is copied except names."}
	if f.review {
		title = config.AppName + " · review accounts to import"
		intro = []string{"tick the accounts to create. nothing is copied except names."}
	}
	var body []string
	for _, line := range intro {
		body = append(body, st.Dim.Render(line))
	}
	body = append(body, "")
	rowsAt := len(body)
	body = append(body, m.firstRunRows(g, st)...)
	focusLine := rowsAt + min(f.focus, f.checkboxCount())
	if f.focus >= f.checkboxCount() {
		focusLine = len(body) + 1
	}
	body = append(body, "", m.firstRunButtons(g, st))
	if m.status.text != "" {
		status := m.status.text
		if m.status.level == levelError {
			status = st.Error.Render(status)
		}
		body = append(body, "", status)
	}

	box := renderModal(modalSpec{
		title: title, body: body, footer: m.firstRunHint(g, modalInnerWidth(m.width, false)),
		focus: focusLine, full: true,
	}, m.width, m.height, g, st)
	margin := strings.Repeat(" ", max((m.width-modalBoxWidth(m.width, false))/2, 0))
	for i := range box {
		box[i] = margin + box[i]
	}
	return strings.Join(box, "\n")
}

func (m Model) firstRunHint(g glyphs, width int) string {
	parts := []string{g.UpDown + " move", "space toggle", g.Enter + " choose", "c create", "? keys", "q quit"}
	hint := strings.Join(parts, " "+g.Dot+" ")
	if ansi.StringWidth(hint) > width {
		parts[1], parts[2] = "space", g.Enter
		hint = strings.Join(parts, " "+g.Dot+" ")
	}
	return hint
}

func (m Model) firstRunRows(g glyphs, st styles) []string {
	f := m.firstRun
	cursor := func(i int) string {
		if f.focus == i {
			return g.Caret + " "
		}
		return "  "
	}
	// Unicode terminals get ☑/☐, tinted in the provider colour; ASCII keeps [x].
	box := func(checked bool, color string) string {
		switch {
		case g.Caret == ">":
			if checked {
				return "[x]"
			}
			return "[ ]"
		case checked:
			return tint(color, st.prof, "☑")
		}
		return "☐"
	}
	var rows []string
	if f.review {
		nameWidth := 0
		for _, r := range f.reviewed {
			nameWidth = min(max(nameWidth, ansi.StringWidth(r.item.Name)), 28)
		}
		for i, r := range f.reviewed {
			row := fmt.Sprintf("%s%s %s %s  %s", cursor(i), box(r.checked, r.info.Badge.Color), tint(r.info.Badge.Color, st.prof, st.Bold.Render(r.info.Badge.Text)),
				padCells(truncateCells(r.item.Name, nameWidth, g.Ellipsis), nameWidth), st.Dim.Render(r.item.Target))
			if r.item.Note != "" {
				row += st.Dim.Render("  " + r.item.Note)
			}
			rows = append(rows, row)
		}
		return rows
	}
	for i, s := range f.sources {
		rows = append(rows, fmt.Sprintf("%s%s %s %d %s", cursor(i), box(s.checked, s.info.Badge.Color),
			tint(s.info.Badge.Color, st.prof, st.Bold.Render(s.info.Badge.Text)), s.source.Count, s.source.Noun))
	}
	for _, info := range f.missing {
		cli := info.CLI.Name
		if cli == "" {
			cli = info.Label + " CLI"
		}
		rows = append(rows, "  "+st.Dim.Render(fmt.Sprintf("%s %s %s not installed", g.Missing, info.Badge.Text, cli)))
	}
	return rows
}

func (m Model) firstRunButtons(g glyphs, st styles) string {
	f := m.firstRun
	importButton := renderPrimaryButton(fmt.Sprintf("Import %d", len(f.selected())), f.focus == f.importButton(), true, st)
	if f.review {
		return joinButtons(importButton, renderButton("Back", f.focus == f.secondButton(), true, st))
	}
	return joinButtons(importButton,
		renderButton("Review each", f.focus == f.secondButton(), true, st),
		renderPlainButton("Start empty", f.focus == f.emptyButton(), st))
}
