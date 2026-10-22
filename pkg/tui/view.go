package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/k0wl0n/gctx/pkg/manager"
)

// column widths and names for the three breakpoint tiers.
// tier0: ≥120, tier1: 90–119, tier2: 60–89
type columnSet struct {
	marker, name, project, email, adc, status int
	showEmail, showADC, showDetails           bool
}

func layoutColumns(width int) columnSet {
	switch {
	case width >= 160:
		return columnSet{
			marker: 2, name: 36, project: 36, email: 32, adc: 5, status: 15,
			showEmail: true, showADC: true, showDetails: true,
		}
	case width >= 120:
		return columnSet{
			marker: 2, name: 30, project: 30, email: 26, adc: 5, status: 15,
			showEmail: true, showADC: true, showDetails: true,
		}
	case width >= 90:
		return columnSet{
			marker: 2, name: 28, project: 28, email: 20, status: 16,
			showEmail: true,
		}
	default: // 60–89
		return columnSet{
			marker: 2, name: 30, project: 26, status: 12,
		}
	}
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func pad(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(r))
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// hRule returns a horizontal rule of width w using the glyph character.
func hRule(g glyphs, st styles, w int) string {
	if w <= 0 {
		return ""
	}
	return st.Sep.Render(strings.Repeat(g.HRule, w))
}

// viewMain draws the main screen, including the too-small replacement.
func (m Model) viewMain() string {
	g := newGlyphs(asciiRequested())
	st := newStyles()

	if m.tooSmall() {
		return fmt.Sprintf("Terminal too small: need at least %d×%d, current %d×%d",
			minWidth, minHeight, m.width, m.height)
	}

	cols := layoutColumns(m.width)
	var out strings.Builder

	// Header line
	out.WriteString(renderHeader(m, g, st))
	out.WriteByte('\n')
	out.WriteString(hRule(g, st, m.width))
	out.WriteByte('\n')

	// Table title line (filter or account count)
	out.WriteString(renderTableTitle(m, cols, g, st))
	out.WriteByte('\n')

	// Column headers
	out.WriteString(renderColumnHeaders(cols, st))
	out.WriteByte('\n')

	// Body rows — subtract 2 extra for the two separator lines
	bodyLines := m.height - 7 // header, sep, title, colheaders, sep, status, hints
	if bodyLines < 1 {
		bodyLines = 1
	}

	if m.loadErr != nil {
		msg := fmt.Sprintf("Error loading accounts: %s", m.loadErr)
		out.WriteString(st.Error.Render(truncate(msg, m.width)))
		out.WriteByte('\n')
	} else if len(m.visible) == 0 {
		if m.filter.query != "" {
			out.WriteString(st.Dim.Render(fmt.Sprintf("No accounts match %q", m.filter.query)))
		} else {
			out.WriteString(st.Dim.Render("No accounts configured. Press c to create one."))
		}
		out.WriteByte('\n')
	} else {
		selIdx := m.selectedIndex()
		// scroll so selection is visible
		offset := m.offset
		if selIdx < offset {
			offset = selIdx
		}
		if selIdx >= offset+bodyLines {
			offset = selIdx - bodyLines + 1
		}

		rendered := 0
		for row := offset; row < len(m.visible) && rendered < bodyLines; row++ {
			acct := m.overview.Accounts[m.visible[row]]
			line := renderRow(acct, cols, g, st, acct.Name == m.selected)
			out.WriteString(line)
			out.WriteByte('\n')
			rendered++
		}
		// fill remaining body with blank lines
		for rendered < bodyLines {
			out.WriteByte('\n')
			rendered++
		}
	}

	// Separator before status
	out.WriteString(hRule(g, st, m.width))
	out.WriteByte('\n')

	// Status line
	out.WriteString(renderStatus(m, st, g))
	out.WriteByte('\n')

	// Key-hint line
	out.WriteString(renderHints(m, cols, st))

	return out.String()
}

func renderHeader(m Model, g glyphs, st styles) string {
	var parts []string
	parts = append(parts, st.Title.Render("gctx"))
	if m.version != "" {
		parts = append(parts, st.Dim.Render("v"+m.version))
	}

	// Spacer
	parts = append(parts, " ")

	if m.overview.ShellAccount != "" {
		parts = append(parts, st.Warn.Render("shell: "+m.overview.ShellAccount))
	}

	if active, ok := m.overview.Active(); ok {
		proj := active.ProjectID
		if proj == "" {
			proj = "-"
		}
		parts = append(parts, st.OK.Render(g.Active)+" "+active.Name+" ("+proj+")")
	} else {
		parts = append(parts, st.Dim.Render("no active account"))
	}

	gcfg := m.overview.GcloudActiveConfig
	if gcfg == "" {
		gcfg = "default"
	}
	active, hasActive := m.overview.Active()
	drifted := hasActive && active.ConfigName != gcfg
	gcfgText := st.Dim.Render("cfg:") + " " + gcfg
	if drifted {
		gcfgText += " " + st.Warn.Render(g.Warn+" drift")
	}
	parts = append(parts, gcfgText)

	adcText := renderADCIndicator(m.overview.DefaultADC, g, st)
	parts = append(parts, adcText)

	return strings.Join(parts, "  ")
}

func renderADCIndicator(ind manager.ADCIndicator, g glyphs, st styles) string {
	switch ind {
	case manager.ADCActive:
		return st.Dim.Render("adc:") + " " + st.OK.Render(g.Yes)
	case manager.ADCDiffers:
		return st.Dim.Render("adc:") + " " + st.Warn.Render(g.Warn+" differs")
	case manager.ADCNone:
		return st.Dim.Render("adc:") + " " + st.Error.Render(g.No+" none")
	default:
		return st.Dim.Render("adc: ?")
	}
}

func renderTableTitle(m Model, cols columnSet, g glyphs, st styles) string {
	total := len(m.overview.Accounts)
	if m.filter.query != "" {
		matches := len(m.visible)
		return fmt.Sprintf("filter: %s (%d/%d)", m.filter.query, matches, total)
	}
	return st.Header.Render("Accounts") + fmt.Sprintf(" (%d)", total)
}

func renderColumnHeaders(cols columnSet, st styles) string {
	var b strings.Builder
	b.WriteString(pad("", cols.marker))
	b.WriteString(st.Header.Render(pad("NAME", cols.name)))
	b.WriteString(" ")
	b.WriteString(st.Header.Render(pad("PROJECT", cols.project)))
	if cols.showEmail {
		b.WriteString(" ")
		b.WriteString(st.Header.Render(pad("EMAIL", cols.email)))
	}
	if cols.showADC {
		b.WriteString(" ")
		b.WriteString(st.Header.Render(pad("ADC", cols.adc)))
	}
	b.WriteString(" ")
	b.WriteString(st.Header.Render("STATUS"))
	return b.String()
}

func renderRow(acct manager.AccountStatus, cols columnSet, g glyphs, st styles, selected bool) string {
	// Active marker — colored green
	marker := " "
	if acct.Active {
		marker = st.OK.Render(g.Active)
	}

	// Name — bold+green for active account
	name := truncate(acct.Name, cols.name)
	if acct.Active {
		name = st.ActiveName.Render(pad(name, cols.name))
	} else {
		name = pad(name, cols.name)
	}

	proj := acct.ProjectID
	if proj == "" {
		proj = st.Dim.Render("-")
	} else {
		proj = truncate(proj, cols.project)
	}

	statusText := string(acct.Status)
	statusStyle := healthStyle(acct.Status, st)

	var b strings.Builder
	b.WriteString(pad(marker, cols.marker))
	b.WriteString(name)
	b.WriteString(" ")
	b.WriteString(pad(proj, cols.project))

	if cols.showEmail {
		email := acct.Email
		if email == "" {
			email = st.Dim.Render("-")
		} else {
			email = truncate(email, cols.email)
		}
		b.WriteString(" ")
		b.WriteString(pad(email, cols.email))
	}
	if cols.showADC {
		var adcGlyph string
		if acct.ADCExists {
			adcGlyph = st.OK.Render(g.Yes)
		} else {
			adcGlyph = st.Error.Render(g.No)
		}
		b.WriteString(" ")
		b.WriteString(pad(adcGlyph, cols.adc))
	}
	b.WriteString(" ")
	b.WriteString(statusStyle.Render(truncate(statusText, cols.status)))

	line := b.String()
	if selected {
		line = st.Selected.Render(line)
	}
	return line
}

func healthStyle(h manager.Health, st styles) lipgloss.Style {
	switch h {
	case manager.HealthReady:
		return st.OK
	case manager.HealthError, manager.HealthConfigMissing:
		return st.Error
	case manager.HealthADCMissing, manager.HealthAuthRequired, manager.HealthProjectMismatch:
		return st.Warn
	}
	return lipgloss.NewStyle()
}

func renderStatus(m Model, st styles, g glyphs) string {
	// filter input takes priority
	if m.filter.editing {
		return m.filter.input.View(true)
	}
	if m.busy() {
		return st.Warn.Render(g.Ellipsis + " " + m.spin.View() + " " + m.op.desc)
	}
	if m.status.text == "" {
		return ""
	}
	switch m.status.level {
	case levelError:
		return st.Error.Render(m.status.text)
	case levelWarning:
		return st.Warn.Render(m.status.text)
	default:
		return m.status.text
	}
}

func renderHints(m Model, cols columnSet, st styles) string {
	if !cols.showEmail {
		return st.Dim.Render("↵ switch  ? help  q quit")
	}
	return st.Dim.Render("j/k move  ↵ switch  / filter  d details  l login  c create  s shell  r run  ? help  q quit")
}

// viewOverlay draws the help or details screen.
func (m Model) viewOverlay() string {
	switch m.screen {
	case screenHelp:
		return renderHelp(m)
	case screenDetails:
		return renderDetails(m)
	}
	return ""
}

func renderHelp(m Model) string {
	var b strings.Builder
	b.WriteString("  gctx keybindings\n\n")
	for _, group := range keys.groups() {
		b.WriteString("  " + group.Title + "\n")
		for _, binding := range group.Bindings {
			h := binding.Help()
			line := fmt.Sprintf("    %-20s %s\n", h.Key, h.Desc)
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	b.WriteString("  q / esc  close\n")
	return b.String()
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

// detailsPane renders the right-side details pane for widths ≥ 120.
func (m Model) detailsPane(width int) string {
	acct, ok := m.selectedAccount()
	if !ok {
		return ""
	}
	g := newGlyphs(asciiRequested())
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Name: %s\n", acct.Name))
	b.WriteString(fmt.Sprintf("Project: %s\n", strOrDash(acct.ProjectID)))
	b.WriteString(fmt.Sprintf("Email: %s\n", strOrDash(acct.Email)))
	b.WriteString(fmt.Sprintf("Config: %s\n", acct.ConfigName))
	if acct.ADCExists {
		b.WriteString(g.Yes + " ADC\n")
	} else {
		b.WriteString(g.No + " ADC\n")
	}
	b.WriteString(fmt.Sprintf("Status: %s\n", acct.Status))
	_ = width
	return b.String()
}

// runeLen is imported above; this silences unused import if needed.
var _ = runeLen
