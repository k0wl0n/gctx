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

// viewFirstRun draws the import offer, with the hint line on the last row.
func (m Model) viewFirstRun() string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	f := m.firstRun
	var out []string
	add := func(line string) { out = append(out, "  "+line) }

	if f.review {
		add(st.Title.Render(config.AppName) + st.Dim.Render(" · review accounts to import"))
	} else {
		add(st.Title.Render(config.AppName) + st.Dim.Render(" · no accounts yet"))
	}
	out = append(out, "")
	if f.review {
		add(st.Dim.Render("tick the accounts to create. nothing is copied except names."))
	} else {
		add(st.Dim.Render("found existing CLI setups you can import."))
		add(st.Dim.Render("nothing is copied except names."))
	}
	out = append(out, "")

	rows := m.firstRunRows(g, st)
	room := max(m.height-len(out)-5, 3)
	first := 0
	if f.review && f.focus < f.checkboxCount() && f.focus >= room {
		first = f.focus - room + 1
	}
	if f.review {
		rows = rows[min(first, len(rows)):min(first+room, len(rows))]
	}
	for _, row := range rows {
		out = append(out, truncateCells(row, m.width, g.Ellipsis))
	}
	out = append(out, "")
	out = append(out, truncateCells("  "+m.firstRunButtons(g, st), m.width, g.Ellipsis))

	status := ""
	if m.status.text != "" {
		status = m.status.text
		if m.status.level == levelError {
			status = st.Error.Render(status)
		}
	}
	for len(out) < m.height-2 {
		out = append(out, "")
	}
	out = append(out[:min(len(out), m.height-2)], "  "+status)
	hint := strings.Join([]string{g.UpDown + " move", "space toggle", g.Enter + " choose", "c create", "? keys", "q quit"}, " "+g.Dot+" ")
	out = append(out, st.Dim.Render("  "+hint))
	for i := range out {
		out[i] = truncateCells(out[i], m.width, g.Ellipsis)
	}
	return strings.Join(out, "\n")
}

func (m Model) firstRunRows(g glyphs, st styles) []string {
	f := m.firstRun
	cursor := func(i int) string {
		if f.focus == i {
			return g.Caret + " "
		}
		return "  "
	}
	box := func(checked bool) string {
		if checked {
			return "[x]"
		}
		return "[ ]"
	}
	var rows []string
	if f.review {
		nameWidth := 0
		for _, r := range f.reviewed {
			nameWidth = min(max(nameWidth, ansi.StringWidth(r.item.Name)), 28)
		}
		for i, r := range f.reviewed {
			row := fmt.Sprintf("%s%s %s %s  %s", cursor(i), box(r.checked), tint(r.info.Badge.Color, st.prof, st.Bold.Render(r.info.Badge.Text)),
				padCells(truncateCells(r.item.Name, nameWidth, g.Ellipsis), nameWidth), st.Dim.Render(r.item.Target))
			if r.item.Note != "" {
				row += st.Dim.Render("  " + r.item.Note)
			}
			rows = append(rows, row)
		}
		return rows
	}
	for i, s := range f.sources {
		rows = append(rows, fmt.Sprintf("%s%s %s %d %s", cursor(i), box(s.checked),
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
	button := func(index int, label string) string {
		if f.focus == index {
			return st.TabActive.Render("[ " + label + " ]")
		}
		return "  " + label + "  "
	}
	importLabel := fmt.Sprintf("Import %d", len(f.selected()))
	if f.review {
		return button(f.importButton(), importLabel) + button(f.secondButton(), "Back")
	}
	return button(f.importButton(), importLabel) + button(f.secondButton(), "Review each") + button(f.emptyButton(), "Start empty")
}
