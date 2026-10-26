package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

type paletteSection int

const (
	sectionActions paletteSection = iota
	sectionAccounts
)

// paletteEntry is one palette row. CLI is the equivalent command line, empty
// for TUI-only actions.
type paletteEntry struct {
	Title, CLI, Key string
	Run             func(Model) (Model, tea.Cmd)
	Account         string

	detail   string // dim text after the title, such as a count
	section  paletteSection
	disabled bool
	why      string // shown when a disabled entry is run
	badge    provider.Badge
	health   manager.Health
}

// paletteExempt lists the bindings that have no palette entry, keyed by the
// binding's first key. New features add palette entries first (D16): a
// binding missing from both the palette and this list fails
// TestEveryKeyBindingHasPaletteEntry.
var paletteExempt = map[string]string{
	"j":         "navigation: moves the cursor",
	"k":         "navigation: moves the cursor",
	"g":         "navigation: moves the cursor",
	"G":         "navigation: moves the cursor",
	"ctrl+f":    "navigation: moves the cursor",
	"ctrl+b":    "navigation: moves the cursor",
	"tab":       "navigation: switches the provider tab",
	"shift+tab": "navigation: switches the provider tab",
	"1":         "navigation: jumps to a provider tab",
	">":         "navigation: moves the cursor to the linked account",
	"/":         "filter: needs live typing in the list",
	"esc":       "clears the filter; nothing to run",
	":":         "opens the palette itself",
}

type rowAction struct {
	binding key.Binding
	title   string // %s is the account name
}

// rowActions are the actions on the selected row, in palette order.
var rowActions = []rowAction{
	{keys.Switch, "Activate %s"},
	{keys.Login, "Login %s"},
	{keys.Shell, "Subshell with every active slot"},
	{keys.ShellOne, "Subshell with only %s"},
	{keys.Run, "Run command as %s"},
	{keys.CopyEnv, "Copy env line for %s"},
	{keys.Details, "Show details for %s"},
	{keys.Edit, "Edit %s"},
	{keys.Save, "Save current ADC to %s"},
	{keys.Delete, "Delete %s"},
}

// keyPress builds the message for a binding's key so an entry runs exactly the
// code its key does, gates included.
func keyPress(name string) tea.KeyPressMsg {
	if name == "enter" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	if rest, ok := strings.CutPrefix(name, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: []rune(rest)[0], Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(name)[0], Text: name}
}

func runKey(name string) func(Model) (Model, tea.Cmd) {
	return func(m Model) (Model, tea.Cmd) { return m.updateMain(keyPress(name)) }
}

// paletteEntries lists the actions that apply to the selected row, the global
// actions and a jump entry for every account. Without a selection the row
// actions are listed disabled.
func (m Model) paletteEntries() []paletteEntry {
	acct, hasSelection := m.selectedAccount()
	var entries []paletteEntry

	for _, action := range rowActions {
		first := action.binding.Keys()[0]
		entry := paletteEntry{Key: first, Run: runKey(first), section: sectionActions}
		switch {
		case !hasSelection:
			entry.Title = strings.Replace(action.title, "%s", "selected account", 1)
			entry.disabled, entry.why = true, "No account selected"
		case m.actionBlock(first, acct) != "":
			continue
		default:
			entry.Title = strings.Replace(action.title, "%s", acct.Name, 1)
			entry.Account = acct.Name
			entry.CLI = cliEquivalent(action.binding, acct.Name)
		}
		if !hasSelection {
			entry.CLI = cliEquivalent(action.binding, "<name>")
		}
		entries = append(entries, entry)
	}

	stale := len(m.staleLoginTargets())
	entries = append(entries, paletteEntry{
		Title: "Login all stale accounts", detail: fmt.Sprintf("(%d)", stale), Key: keys.LoginStale.Keys()[0],
		Run: runKey(keys.LoginStale.Keys()[0]), section: sectionActions,
		disabled: stale == 0, why: "no account needs login",
	})
	for _, global := range []struct {
		title   string
		binding key.Binding
	}{
		{"Recheck credentials", keys.Reload},
		{"Show help", keys.Help},
		{"Toggle detail pane", keys.TogglePane},
		{"Fold or unfold provider group", keys.Fold},
		{"Toggle rows needing action", keys.NeedsAction},
		{"Cycle sort order", keys.Sort},
		{"Create account", keys.Create},
		{"Quit", keys.Quit},
	} {
		first := global.binding.Keys()[0]
		entries = append(entries, paletteEntry{
			Title: global.title, Key: first, CLI: cliEquivalent(global.binding, "<name>"),
			Run: runKey(first), section: sectionActions,
		})
	}

	for _, account := range m.overview.Accounts {
		name := account.Name
		entries = append(entries, paletteEntry{
			Title: name, CLI: "jump", Account: name, section: sectionAccounts,
			badge: m.providerInfo(account.Provider).Badge, health: account.Status,
			Run: func(m Model) (Model, tea.Cmd) { return m.jumpTo(name) },
		})
	}
	return entries
}

// jumpTo selects name, widening the view to everything when it is hidden.
func (m Model) jumpTo(name string) (Model, tea.Cmd) {
	m.feedback = nil
	if !m.isVisible(name) {
		m.filter.query = ""
		m.filter.input.Reset()
		m.needsAction = false
		m.tab = 0
		if account, ok := m.overview.Account(name); ok {
			m.folded = cloneOrNew(m.folded)
			delete(m.folded, string(account.Provider))
		}
		m.refilter()
	}
	if row := m.visibleIndex(name); row >= 0 {
		m = m.moveToIndex(row)
	}
	return m, nil
}

// ─── Fuzzy matching ─────────────────────────────────────────────────────────

const (
	scoreMatch       = 1
	scoreStart       = 4
	scoreWordStart   = 3
	scoreConsecutive = 5
	gapPenaltyMax    = 5
)

func isWordBoundary(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// fuzzyMatch scores query as a case-insensitive subsequence of text, rewarding
// consecutive matches and matches at word starts, and returns the matched rune
// positions of the best alignment.
func fuzzyMatch(query, text string) (score int, positions []int, ok bool) {
	q, t := []rune(strings.ToLower(query)), []rune(text)
	lower := []rune(strings.ToLower(text))
	if len(q) == 0 {
		return 0, nil, true
	}
	if len(q) > len(t) || len(lower) != len(t) {
		return 0, nil, false
	}
	const none = -1 << 30
	best := make([][]int, len(q))
	from := make([][]int, len(q))
	for j := range q {
		best[j], from[j] = make([]int, len(t)), make([]int, len(t))
		for i := range t {
			best[j][i], from[j][i] = none, -1
			if lower[i] != q[j] {
				continue
			}
			gain := scoreMatch
			switch {
			case i == 0:
				gain += scoreStart
			case isWordBoundary(t[i-1]):
				gain += scoreWordStart
			}
			if j == 0 {
				best[j][i] = gain
				continue
			}
			for k := range i {
				if best[j-1][k] == none {
					continue
				}
				step := -min(i-k-1, gapPenaltyMax)
				if k == i-1 {
					step = scoreConsecutive
				}
				if candidate := best[j-1][k] + gain + step; candidate > best[j][i] {
					best[j][i], from[j][i] = candidate, k
				}
			}
		}
	}
	end, top := -1, none
	for i := range t {
		if best[len(q)-1][i] > top {
			end, top = i, best[len(q)-1][i]
		}
	}
	if end < 0 {
		return 0, nil, false
	}
	positions = make([]int, len(q))
	for j, i := len(q)-1, end; j >= 0; j-- {
		positions[j] = i
		i = from[j][i]
	}
	return top, positions, true
}

type rankedEntry struct {
	entry     paletteEntry
	positions []int
	score     int
}

// rankMatches filters entries by query and orders them by score, actions
// before accounts; an empty query keeps every entry in its natural order.
func rankMatches(query string, entries []paletteEntry) []rankedEntry {
	var ranked []rankedEntry
	for _, entry := range entries {
		score, positions, ok := fuzzyMatch(query, entry.Title)
		if ok {
			ranked = append(ranked, rankedEntry{entry: entry, positions: positions, score: score})
		}
	}
	slices.SortStableFunc(ranked, func(a, b rankedEntry) int {
		if a.entry.section != b.entry.section {
			return int(a.entry.section) - int(b.entry.section)
		}
		return b.score - a.score
	})
	return ranked
}

func rankEntries(query string, entries []paletteEntry) []paletteEntry {
	var out []paletteEntry
	for _, ranked := range rankMatches(query, entries) {
		out = append(out, ranked.entry)
	}
	return out
}

// ─── Overlay ────────────────────────────────────────────────────────────────

type palette struct {
	entries []paletteEntry
	input   lineInput
	shown   []rankedEntry
	cursor  int
}

func newPalette(m Model) *palette {
	p := &palette{entries: m.paletteEntries(), input: newLineInput("")}
	p.refresh()
	return p
}

func (p *palette) refresh() {
	p.shown = rankMatches(p.input.Value(), p.entries)
	p.cursor = 0
}

func (p *palette) move(delta int) {
	if len(p.shown) > 0 {
		p.cursor = (p.cursor + delta + len(p.shown)) % len(p.shown)
	}
}

func (p *palette) update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	m.modal = p
	switch press.String() {
	case "esc":
		m.modal = nil
	case "up", "ctrl+p":
		p.move(-1)
	case "down", "ctrl+n":
		p.move(1)
	case "tab":
		if len(p.shown) > 0 {
			p.input.SetValue(p.shown[p.cursor].entry.Title)
			p.refresh()
		}
	case "enter":
		if len(p.shown) == 0 {
			return m, nil
		}
		entry := p.shown[p.cursor].entry
		m.modal = nil
		if entry.disabled {
			return m, m.setStatus(entry.why, levelInfo)
		}
		return entry.Run(m)
	default:
		if p.input.update(press) {
			p.refresh()
		}
	}
	return m, nil
}

type paletteLine struct {
	text string
	row  int // index into shown, -1 for headers and blanks
}

func (p *palette) view(m Model) string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	inner := modalInnerWidth(max(m.width, minWidth), true)

	const keyWidth = 5
	cliWidth := min(36, inner*2/5)
	titleWidth := inner - 1 - 4 - cliWidth - keyWidth

	var body []paletteLine
	section := paletteSection(-1)
	for i, ranked := range p.shown {
		entry := ranked.entry
		if entry.section != section {
			if len(body) > 0 {
				body = append(body, paletteLine{row: -1})
			}
			header := "actions"
			if entry.section == sectionAccounts {
				header = "accounts"
			}
			body = append(body, paletteLine{text: st.Dim.Render(header), row: -1})
			section = entry.section
		}
		body = append(body, paletteLine{text: p.rowText(ranked, i == p.cursor, titleWidth, cliWidth, keyWidth, g, st), row: i})
	}
	if len(p.shown) == 0 {
		body = append(body, paletteLine{text: st.Dim.Render("no matches"), row: -1})
	}

	focus := slices.IndexFunc(body, func(l paletteLine) bool { return l.row == p.cursor })
	count := fmt.Sprintf("%d matches", len(p.shown))
	if len(p.shown) == 1 {
		count = "1 match"
	}
	var lines []string
	for _, line := range body {
		lines = append(lines, line.text)
	}

	hint := strings.Join([]string{g.UpDown + " select", g.Enter + " run", g.Tab + " complete", "esc close"}, "  ")
	const hintRight = "right column = the CLI equivalent"
	if ansi.StringWidth(hint)+2+len(hintRight) <= inner {
		hint += strings.Repeat(" ", inner-ansi.StringWidth(hint)-len(hintRight)) + hintRight
	}
	return m.showModal(modalSpec{title: ": " + p.input.View(true), hint: count, body: lines, footer: hint, focus: focus, wide: true})
}

func (p *palette) rowText(ranked rankedEntry, selected bool, titleWidth, cliWidth, keyWidth int, g glyphs, st styles) string {
	entry := ranked.entry
	title := highlight(entry.Title, ranked.positions, st)
	if entry.disabled {
		title = st.Dim.Render(entry.Title)
	}
	if entry.detail != "" {
		title += " " + st.Warn.Render(entry.detail)
	}
	if entry.section == sectionAccounts {
		title = renderBadge(entry.badge, st.prof, g) + " " + title
		if entry.health != manager.HealthReady && entry.health != "" {
			title += " " + healthStyle(entry.health, st).Render(healthGlyph(entry.health, g))
		}
	}
	cli := entry.CLI
	switch {
	case entry.section == sectionAccounts:
	case cli == "":
		cli = "(tui)"
	}
	keyText := entry.Key
	if keyText == "enter" {
		keyText = g.Enter
	}
	bar := " "
	if selected {
		bar = g.Bar
	}
	line := bar + padTo(title, titleWidth) + "  " + st.Dim.Render(padTo(cli, cliWidth)) + "  " + st.Dim.Render(fmt.Sprintf("%*s", keyWidth, keyText))
	if selected {
		line = st.Selected.Render(line)
	}
	return line
}

// highlight draws text with the runes at positions in bold.
func highlight(text string, positions []int, st styles) string {
	if len(positions) == 0 {
		return text
	}
	var b strings.Builder
	for i, r := range []rune(text) {
		if slices.Contains(positions, i) {
			b.WriteString(st.Bold.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// padTo truncates s to width cells and pads it with spaces.
func padTo(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}
