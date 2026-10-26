package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/provider"

	"github.com/k0wl0n/gctx/pkg/manager"
)

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// hRule returns a horizontal rule of width w using the glyph character.
func hRule(g glyphs, st styles, w int) string {
	if w <= 0 {
		return ""
	}
	return st.Sep.Render(strings.Repeat(g.HRule, w))
}

// viewMain draws the main screen, including the too-small replacement.
// Regions, top to bottom: header, tab row, list (plus the detail pane when
// wide), status line, hint line.
func (m Model) viewMain() string {
	if m.tooSmall() {
		return fmt.Sprintf("Terminal too small: need at least %d×%d, current %d×%d",
			minWidth, minHeight, m.width, m.height)
	}
	if m.firstRun != nil {
		return m.viewFirstRun()
	}
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	t := tierFor(m.width)

	top := m.headerLines(t, g, st)
	if t != tierNarrow {
		top = append(top, m.tabRow(g, st))
	}
	top = append(top, hRule(g, st, m.width))
	if m.offline {
		top = append(top, m.offlineBanner(m.width, g, st))
	}
	bodyHeight := max(m.height-len(top)-4, 1) // column headers, rule, status, hints
	var panel []string
	if m.feedback != nil && m.panelFits() {
		panel = m.feedbackLines(m.width, g, st)
		bodyHeight -= len(panel)
	}

	listWidth := m.width
	showPane := t == tierWide && m.paneOpen && !m.onMissingCLITab()
	if showPane {
		listWidth = m.width - paneWidth(m.width) - 3
	}
	var list []string
	if m.onMissingCLITab() {
		list = m.missingCLILines(listWidth, bodyHeight+1, g, st)
	} else {
		cols := m.tableColumns(listWidth - markerWidth)
		list = append([]string{renderColumnHeaders(cols, g, st)}, m.listLines(cols, listWidth, bodyHeight, g, st)...)
	}

	if showPane {
		pane := m.paneLines(m.width-listWidth-3, g, st)
		sep := st.Sep.Render(g.VSep)
		for i := range list {
			right := ""
			if i < len(pane) {
				right = pane[i]
			}
			list[i] = padCells(list[i], listWidth) + " " + sep + " " + right
		}
	}

	out := append(top, list...)
	out = append(out, panel...)
	out = append(out,
		hRule(g, st, m.width),
		m.renderStatus(m.width, g, st),
		renderHints(m, m.width, st),
	)
	return strings.Join(out, "\n")
}

// listLines is the list body, exactly height lines, scrolled so the
// selected row is visible. On All it is grouped by category and provider.
func (m Model) listLines(cols []colSpec, width, height int, g glyphs, st styles) []string {
	var body []string
	selectedLine := 0
	switch {
	case m.loadErr != nil:
		body = append(body, st.Error.Render(truncateCells("Error loading accounts: "+m.loadErr.Error(), width, g.Ellipsis)))
	case len(m.groups) == 0:
		text := "No accounts configured. Press c to create one."
		info, onTab := m.tabProvider()
		switch {
		case m.loadErr == nil && onTab && m.filter.query == "" && !m.needsAction:
			text = fmt.Sprintf("No %s accounts yet. Press c to create one.", info.Label)
		case m.filter.query != "":
			text = fmt.Sprintf("No accounts match %q", m.filter.query)
		case m.needsAction:
			text = "Nothing needs action"
		}
		body = append(body, st.Dim.Render(truncateCells(text, width, g.Ellipsis)))
	default:
		_, onTab := m.tabProvider()
		headings := !onTab && tierFor(m.width) != tierNarrow
		var category provider.Category
		for i, group := range m.groups {
			if headings && (i == 0 || group.info.Category != category) {
				if i > 0 {
					body = append(body, "")
				}
				body = append(body, m.categoryHeading(group.info.Category, width, g, st))
			}
			category = group.info.Category
			if headings {
				body = append(body, m.groupHeading(group.info, width, g, st))
			}
			for _, idx := range group.rows {
				acct := m.overview.Accounts[idx]
				if acct.Name == m.selected {
					selectedLine = len(body)
				}
				body = append(body, m.renderRow(acct, cols, g, st, acct.Name == m.selected, width))
			}
			if group.hidden > 0 {
				body = append(body, m.placeholderLine(group, width, g, st))
				if m.selected == foldKey(group.info.ID) {
					selectedLine = len(body) - 1
				}
			}
		}
	}
	offset := 0
	if selectedLine >= height {
		offset = selectedLine - height + 1
	}
	body = body[min(offset, len(body)):]
	if len(body) > height {
		body = body[:height]
	}
	for len(body) < height {
		body = append(body, "")
	}
	return body
}

// placeholderLine is a folded group's selectable "… N more ready" row.
func (m Model) placeholderLine(group listGroup, width int, g glyphs, st styles) string {
	text := fmt.Sprintf("%s %d more ready %s %s expand", g.Ellipsis, group.hidden, g.Dot, keys.Fold.Keys()[0])
	if m.selected != foldKey(group.info.ID) {
		return st.Dim.Render(fitCells("  "+text, width, g.Ellipsis))
	}
	return st.Selected.Render(fitCells(g.Bar+" "+text, width, g.Ellipsis))
}

func (m Model) categoryHeading(c provider.Category, width int, g glyphs, st styles) string {
	total := 0
	for _, info := range m.listedProviders() {
		if info.Category == c {
			total += info.Count
		}
	}
	return ruledLine(st.Bold.Render(categoryHeading(c)), fmt.Sprint(total), width, g, st)
}

// groupHeading is a provider's heading: label, target label and summary.
func (m Model) groupHeading(info manager.ProviderInfo, width int, g glyphs, st styles) string {
	left := tint(info.Badge.Color, st.prof, strings.ToUpper(info.Label)) + st.Dim.Render(" "+g.Dot+" "+strings.ToLower(info.TargetLabel))
	return ruledLine(left, m.groupSummary(info, g), width, g, st)
}

// groupSummary counts a provider's accounts by state: "9 · 7 ready, 1 expired, 1 login".
func (m Model) groupSummary(info manager.ProviderInfo, g glyphs) string {
	labels := []string{"ready", "expired", "login", "unverified", "low budget", "n/a", "issue"}
	counts := map[string]int{}
	total := 0
	for _, a := range m.overview.Accounts {
		if a.Provider != info.ID {
			continue
		}
		total++
		switch a.Status {
		case manager.HealthReady:
			counts["ready"]++
		case manager.HealthExpired:
			counts["expired"]++
		case manager.HealthAuthRequired, manager.HealthADCMissing:
			counts["login"]++
		case manager.HealthUnverified:
			counts["unverified"]++
		case manager.HealthBudgetLow:
			counts["low budget"]++
		case manager.HealthCLIMissing:
			counts["n/a"]++
		default:
			counts["issue"]++
		}
	}
	var parts []string
	for _, label := range labels {
		if n := counts[label]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	return fmt.Sprintf("%d %s %s", total, g.Dot, strings.Join(parts, ", "))
}

// ruledLine is "left ──── right" across width.
func ruledLine(left, right string, width int, g glyphs, st styles) string {
	fill := width - ansi.StringWidth(left) - ansi.StringWidth(right) - 2
	if fill < 1 {
		return truncateCells(left+" "+right, width, g.Ellipsis)
	}
	return left + " " + st.Sep.Render(strings.Repeat(g.HRule, fill)) + " " + st.Dim.Render(right)
}

// viewOverlay draws the help, details or login screen.
func (m Model) viewOverlay() string {
	switch m.screen {
	case screenHelp:
		return m.viewHelp()
	case screenDetails:
		return renderDetails(m)
	case screenLogin:
		return m.viewLogin()
	}
	return ""
}

func renderDetails(m Model) string {
	acct, ok := m.selectedAccount()
	if !ok {
		return "No account selected"
	}
	g := newGlyphs(asciiRequested())
	var b strings.Builder
	b.WriteString(fmt.Sprintf("  Account: %s\n", acct.Name))
	b.WriteString(fmt.Sprintf("  Project: %s\n", strOrDash(acct.ProjectID)))
	b.WriteString(fmt.Sprintf("  Email:   %s\n", strOrDash(acct.Email)))
	b.WriteString(fmt.Sprintf("  Active:  %v\n", acct.Active))
	b.WriteByte('\n')
	b.WriteString(fmt.Sprintf("  gcloud config:    %s (exists: %v)\n", acct.ConfigName, acct.GcloudConfigExists))
	b.WriteString(fmt.Sprintf("  config account:   %s\n", strOrDash(acct.GcloudAccount)))
	b.WriteString(fmt.Sprintf("  config project:   %s\n", strOrDash(acct.GcloudProject)))
	b.WriteByte('\n')
	adcPath := acct.StoredADCPath
	if adcPath == "" {
		adcPath = acct.ADCPath
	}
	b.WriteString(fmt.Sprintf("  Stored ADC path:  %s\n", strOrDash(adcPath)))
	b.WriteString(fmt.Sprintf("  ADC exists:       %v\n", acct.ADCExists))
	if acct.ADCExists && !acct.ADCModTime.IsZero() {
		b.WriteString(fmt.Sprintf("  ADC modified:     %s\n", acct.ADCModTime.Format(time.RFC3339)))
	}
	if !acct.CreatedAt.IsZero() {
		b.WriteString(fmt.Sprintf("  Created:          %s\n", acct.CreatedAt.Format(time.RFC3339)))
	}
	b.WriteByte('\n')
	if len(acct.Findings) == 0 {
		b.WriteString("  " + g.Yes + " No issues found\n")
	} else {
		b.WriteString("  Findings:\n")
		for _, f := range acct.Findings {
			remedy := findingRemedy(f)
			b.WriteString(fmt.Sprintf("    %s — %s\n", f.Health, remedy))
		}
	}
	if len(acct.Details) > 0 {
		b.WriteString("\n  Provider details:\n")
		for _, field := range acct.Details {
			b.WriteString(fmt.Sprintf("    %-16s %s\n", field.Label+":", strOrDash(field.Value)))
		}
	}
	if env := m.envs[acct.Name]; len(env) > 0 {
		b.WriteString("\n  Env applied on activate:\n")
		for _, kv := range env {
			b.WriteString("    " + displayEnv(kv) + "\n")
		}
	}
	b.WriteString("\n  esc/d/i/q  close\n")
	return b.String()
}

func findingRemedy(f manager.Finding) string {
	switch f.Health {
	case manager.HealthADCMissing:
		return "press l to log in, or a to save the current ADC"
	case manager.HealthAuthRequired:
		return "press l to log in"
	case manager.HealthConfigMissing:
		return "delete with ctrl+d and recreate with c"
	case manager.HealthProjectMismatch:
		return "press enter to re-apply the project"
	case manager.HealthError:
		return f.Message
	}
	return string(f.Health)
}

func strOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
