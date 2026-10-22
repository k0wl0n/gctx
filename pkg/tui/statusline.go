package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// renderStatus is the status line: the filter input, the running operation
// or a transient message on the left (credential check progress when there
// is none), and account totals on the right.
func (m Model) renderStatus(width int, g glyphs, st styles) string {
	right := st.Dim.Render(m.totals(g, false))
	if width-ansi.StringWidth(right) < 40 {
		right = st.Dim.Render(m.totals(g, true))
	}
	budget := width - ansi.StringWidth(right) - 2
	var left string
	switch {
	case m.filter.editing:
		left = m.filter.input.View(true)
	case m.busy():
		left = st.Warn.Render(m.spinnerFrame() + " " + m.op.desc)
	case m.status.text != "":
		switch m.status.level {
		case levelError:
			left = st.Error.Render(m.status.text)
		case levelWarning:
			left = st.Warn.Render(m.status.text)
		default:
			left = m.status.text
		}
	default:
		left = m.progress(budget, g, st)
	}
	if budget < 20 {
		return truncateCells(left, width, g.Ellipsis)
	}
	left = truncateCells(left, budget, g.Ellipsis)
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-ansi.StringWidth(right)) + right
}

// totals counts the accounts that need attention, over every account;
// short gives one count of everything that needs action.
func (m Model) totals(g glyphs, short bool) string {
	if short {
		n := 0
		for _, a := range m.overview.Accounts {
			if needsAction(a) {
				n++
			}
		}
		return fmt.Sprintf("%d accounts %s %d need action", len(m.overview.Accounts), g.Dot, n)
	}
	counts := map[string]int{}
	for _, a := range m.overview.Accounts {
		switch a.Status {
		case manager.HealthExpired:
			counts["expired"]++
		case manager.HealthAuthRequired, manager.HealthADCMissing:
			counts["need login"]++
		case manager.HealthBudgetLow:
			counts["low budget"]++
		case manager.HealthUnverified:
			counts["unverified"]++
		}
	}
	parts := []string{fmt.Sprintf("%d accounts", len(m.overview.Accounts))}
	for _, label := range []string{"expired", "need login", "low budget", "unverified"} {
		if n := counts[label]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	return strings.Join(parts, " "+g.Dot+" ")
}

// progressItem is one entry of the check progress.
type progressItem struct {
	text, short string // short omits the timeout
	category    provider.Category
	ok          bool // finished without warnings
	checked     int
	total       int
}

// progress renders per-provider check progress, as compact as it must be
// to fit: finished categories collapse into one entry, then every cleanly
// finished provider does, then the timeout is shown once, then the line is cut.
func (m Model) progress(budget int, g glyphs, st styles) string {
	var items []progressItem
	for _, info := range m.listedProviders() {
		p, ok := m.creds.progress[info.ID]
		if !ok {
			continue
		}
		item := progressItem{category: info.Category, checked: p.checked, total: p.total}
		count := fmt.Sprintf("%s %d/%d", info.Label, p.checked, p.total)
		switch {
		case !p.done:
			item.short = m.spinnerFrame() + " " + count
			item.text = item.short + fmt.Sprintf(" (timeout %s)", p.timeout)
		case p.failed:
			item.text = st.Error.Render(g.No + " " + info.Label + " check failed")
		case p.warning != "":
			item.text = st.Warn.Render(g.Unverified + " " + count + " unverified")
		default:
			item.ok, item.text = true, st.OK.Render(g.Yes)+" "+count
		}
		if item.short == "" {
			item.short = item.text
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		if m.creds.phase == credFailed && m.creds.err != nil {
			return st.Error.Render("credential check failed: " + m.creds.err.Error())
		}
		return ""
	}
	full := joinItems(items, false)
	if ansi.StringWidth(full) <= budget {
		return full
	}
	items = collapseFinished(items, g, st)
	if line := joinItems(items, false); ansi.StringWidth(line) <= budget {
		return line
	}
	items = mergeFinished(items, g, st)
	if line := joinItems(items, false); ansi.StringWidth(line) <= budget {
		return line
	}
	line := joinItems(items, true)
	for _, info := range m.listedProviders() {
		if p, ok := m.creds.progress[info.ID]; ok && !p.done {
			line += fmt.Sprintf(" (timeout %s)", p.timeout)
			break
		}
	}
	return line
}

func joinItems(items []progressItem, short bool) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = item.text
		if short {
			parts[i] = item.short
		}
	}
	return strings.Join(parts, "  ")
}

// collapseFinished merges each category whose providers all finished
// cleanly into one "✓ clouds 12/12" entry.
func collapseFinished(items []progressItem, g glyphs, st styles) []progressItem {
	var out []progressItem
	for i := 0; i < len(items); {
		j := i
		allOK := true
		checked, total := 0, 0
		for j < len(items) && items[j].category == items[i].category {
			allOK = allOK && items[j].ok
			checked += items[j].checked
			total += items[j].total
			j++
		}
		if allOK && j-i > 1 {
			text := st.OK.Render(g.Yes) + fmt.Sprintf(" %s %d/%d", categoryShort(items[i].category), checked, total)
			out = append(out, progressItem{text: text, short: text, category: items[i].category, ok: true, checked: checked, total: total})
		} else {
			out = append(out, items[i:j]...)
		}
		i = j
	}
	return out
}

// mergeFinished folds every cleanly finished entry into one leading entry.
func mergeFinished(items []progressItem, g glyphs, st styles) []progressItem {
	var rest []progressItem
	checked, total, merged := 0, 0, 0
	for _, item := range items {
		if item.ok {
			checked, total, merged = checked+item.checked, total+item.total, merged+1
			continue
		}
		rest = append(rest, item)
	}
	if merged == 0 {
		return items
	}
	text := st.OK.Render(g.Yes) + fmt.Sprintf(" %d/%d checked", checked, total)
	return append([]progressItem{{text: text, short: text, ok: true, checked: checked, total: total}}, rest...)
}

func categoryShort(c provider.Category) string {
	switch c {
	case provider.CategoryCloud:
		return "clouds"
	case provider.CategoryAITool:
		return "AI tools"
	case provider.CategoryAIGateway:
		return "gateways"
	}
	return string(c)
}

// renderHints is the hint line generated from keyMap, keeping the entries
// that fit width.
func renderHints(m Model, width int, st styles) string {
	g := newGlyphs(asciiRequested())
	_, onTab := m.tabProvider()
	var parts []string
	used := 0
	for _, h := range keys.hints(tierFor(width), !onTab, g) {
		entry := h.key + " " + st.Dim.Render(h.label)
		w := ansi.StringWidth(entry)
		if len(parts) > 0 {
			w += 2
		}
		if used+w > width {
			break
		}
		parts = append(parts, entry)
		used += w
	}
	return strings.Join(parts, "  ")
}
