package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// slotView is one header slot (D9): the providers sharing a slot get one
// entry, labelled by the provider of its active account.
type slotView struct {
	badge    provider.Badge
	category provider.Category
	active   string // "" when the slot has accounts but none is active
}

func (m Model) slots() []slotView {
	var out []slotView
	index := map[string]int{}
	for _, info := range m.listedProviders() {
		if info.Count == 0 {
			continue // no accounts, no slot
		}
		slot := info.Slot
		if slot == "" {
			slot = string(info.ID)
		}
		if _, seen := index[slot]; seen {
			continue
		}
		view := slotView{badge: info.Badge, category: info.Category, active: m.overview.ActiveBySlot[slot]}
		if account, ok := m.overview.Account(view.active); ok {
			view.badge = m.providerInfo(account.Provider).Badge
		} else {
			view.active = ""
		}
		index[slot] = len(out)
		out = append(out, view)
	}
	return out
}

// headerLines are the app line and, outside the narrow tier, the env line.
func (m Model) headerLines(t tier, g glyphs, st styles) []string {
	title := st.Title.Render(config.AppName)
	if t == tierNarrow {
		var dots []string
		active := 0
		for _, s := range m.slots() {
			glyph := g.Missing
			if s.active != "" {
				glyph = g.Active
				active++
			}
			dots = append(dots, tint(s.badge.Color, st.prof, glyph))
		}
		left := title + " " + strings.Join(dots, " ") + st.Dim.Render(fmt.Sprintf("  %d active", active))
		return []string{joinSides(left, "tab "+m.tabChip(m.tab, g, st), m.width, g)}
	}

	// The standard tier drops the version and the active dots to make room.
	left := title
	if m.version != "" && t == tierWide {
		left += " " + st.Dim.Render("v"+m.version)
	}
	left += " " + st.Dim.Render(g.VSep)
	previous := provider.Category("")
	for i, s := range m.slots() {
		sep := "  "
		if i == 0 {
			sep = " "
		}
		if i > 0 && previous == provider.CategoryCloud && s.category != provider.CategoryCloud {
			sep = " " + st.Dim.Render(g.VSep+" ai") + " "
		}
		previous = s.category
		part := tint(s.badge.Color, st.prof, st.Bold.Render(s.badge.Text)) + " "
		switch {
		case s.active == "":
			part += tint(s.badge.Color, st.prof, g.Missing)
		case t == tierWide:
			part += tint(s.badge.Color, st.prof, g.Active) + " " + s.active
		default:
			part += s.active
		}
		left += sep + part
	}
	return []string{
		joinSides(left, m.headerHealth(t, g, st), m.width, g),
		st.Dim.Render(truncateCells("env "+m.envLine(g), m.width, g.Ellipsis)),
	}
}

// headerHealth is config health, the shell account and the last check's age.
func (m Model) headerHealth(t tier, g glyphs, st styles) string {
	var parts []string
	if m.overview.ShellAccount != "" {
		parts = append(parts, st.Warn.Render("shell: "+m.overview.ShellAccount))
	}
	active, hasActive := m.overview.Active()
	switch {
	case m.loadErr != nil:
		parts = append(parts, st.Error.Render(g.No+" cfg"))
	case hasActive && m.overview.GcloudActiveConfig != "" && active.ConfigName != m.overview.GcloudActiveConfig:
		parts = append(parts, st.Warn.Render(g.Warn+" cfg drift"))
	default:
		parts = append(parts, st.OK.Render(g.Yes+" cfg"))
	}
	if m.overview.DefaultADC == manager.ADCDiffers {
		parts = append(parts, st.Warn.Render(g.Warn+" adc differs"))
	}
	if age, ok := m.checkAge(); ok {
		text := formatAge(age)
		if t == tierWide {
			text = "auth checked " + text + " ago"
		}
		parts = append(parts, st.Dim.Render(text))
	}
	return strings.Join(parts, "  ")
}

// checkAge is the age of the last finished check, or of the newest cached
// result before the first check of this session finishes.
func (m Model) checkAge() (time.Duration, bool) {
	last := m.creds.finished
	if last.IsZero() {
		for _, a := range m.overview.Accounts {
			if a.Checked.After(last) {
				last = a.Checked
			}
		}
	}
	if last.IsZero() {
		return 0, false
	}
	return max(m.now().Sub(last), 0), true
}

// envLine is what the active accounts' slots would export.
func (m Model) envLine(g glyphs) string {
	var entries []string
	for _, s := range m.slots() {
		if s.active == "" {
			continue
		}
		for _, kv := range m.envs[s.active] {
			entries = append(entries, displayEnv(kv))
		}
	}
	if len(entries) == 0 {
		return "–"
	}
	return strings.Join(entries, " ")
}

// keyRefSchemes are the D13 key reference forms. A reference is shown as
// ‹ref›; it is a lookup, never the key.
var keyRefSchemes = []string{"op://", "gcpsm://", "keychain:", "env:"}

// secretNameParts mark a variable whose value must never be shown unless it
// is a key reference.
var secretNameParts = []string{"KEY", "TOKEN", "SECRET", "PASSWORD"}

// displayEnv renders one NAME=value env entry for the screen.
func displayEnv(kv string) string {
	name, value, ok := strings.Cut(kv, "=")
	if !ok {
		return kv
	}
	for _, scheme := range keyRefSchemes {
		if strings.HasPrefix(value, scheme) {
			return name + "=‹" + value + "›"
		}
	}
	upper := strings.ToUpper(name)
	for _, part := range secretNameParts {
		if strings.Contains(upper, part) {
			return name + "=‹hidden›"
		}
	}
	return kv
}

type tabView struct {
	label    string
	count    int
	color    string
	category provider.Category
	missing  bool // CLI not on PATH
}

func (m Model) tabViews() []tabView {
	tabs := []tabView{{label: "All", count: len(m.overview.Accounts)}}
	for _, info := range m.listedProviders() {
		tabs = append(tabs, tabView{label: info.Label, count: info.Count, color: info.Badge.Color, category: info.Category, missing: !info.CLI.Found})
	}
	return tabs
}

// tabChip draws one tab; the current tab is reverse video, or bracketed
// without colour.
func (m Model) tabChip(i int, g glyphs, st styles) string {
	tabs := m.tabViews()
	if i < 0 || i >= len(tabs) {
		return ""
	}
	tab := tabs[i]
	text := fmt.Sprintf("%s %d", tab.label, tab.count)
	if tab.missing {
		text = tab.label + " n/a"
	}
	if i == m.tab {
		if st.prof <= colorprofile.ASCII {
			return "[" + text + "]"
		}
		return st.TabActive.Render(" " + text + " ")
	}
	switch {
	case tab.missing:
		text = st.Dim.Render(text)
	case tab.color != "":
		text = tint(tab.color, st.prof, tab.label) + fmt.Sprintf(" %d", tab.count)
	}
	return " " + text + " "
}

// tabRow is the tab strip with the sort and filter state on the right.
func (m Model) tabRow(g glyphs, st styles) string {
	var b strings.Builder
	tabs := m.tabViews()
	for i := range tabs {
		if i > 1 && tabs[i].category != tabs[i-1].category {
			b.WriteString(st.Dim.Render(g.VSep))
		}
		b.WriteString(m.tabChip(i, g, st))
	}
	var right []string
	if m.needsAction {
		right = append(right, st.Warn.Render("! needs action"))
	}
	right = append(right, st.Dim.Render("sort")+" "+m.sort.String())
	if m.filter.query != "" {
		right = append(right, fmt.Sprintf("filter: %s (%d/%d)", m.filter.query, len(m.visible), len(m.overview.Accounts)))
	} else {
		right = append(right, st.Dim.Render("/ filter"))
	}
	return joinSides(b.String(), strings.Join(right, "  "), m.width, g)
}

// joinSides puts right flush right after left, cutting left when both do
// not fit and dropping right when even that is not enough.
func joinSides(left, right string, width int, g glyphs) string {
	rw := ansi.StringWidth(right)
	if rw+2 > width/2 {
		return truncateCells(left, width, g.Ellipsis)
	}
	left = truncateCells(left, width-rw-2, g.Ellipsis)
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-rw) + right
}
