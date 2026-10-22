package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
)

// markerWidth is the cursor bar plus the active dot.
const markerWidth = 2

// Content-based widths are capped so one long value cannot push other
// columns off screen.
var columnCaps = map[string]int{
	colName: 22, colTarget: 24, colIdentity: 30, colStatus: 20, colExpires: 10,
}

// tableColumns sizes the visible columns from the visible rows and fits them
// into width.
func (m Model) tableColumns(width int) []colSpec {
	g := newGlyphs(asciiRequested())
	cols := []colSpec{
		{key: colName, header: "NAME", minWidth: 8},
		{key: colTarget, header: "TARGET", minWidth: 8},
		{key: colIdentity, header: "IDENTITY", priority: dropIdentity},
	}
	if m.anyVisible(adcApplies) {
		cols = append(cols, colSpec{key: colADC, header: "ADC", priority: dropADC})
	}
	cols = append(cols,
		colSpec{key: colStatus, header: "STATUS"},
		colSpec{key: colExpires, header: m.expiresHeader(), priority: dropExpires},
	)
	for i := range cols {
		cols[i].width = ansi.StringWidth(cols[i].header)
		for _, idx := range m.visible {
			cell := m.cellText(m.overview.Accounts[idx], cols[i].key, g)
			cols[i].width = max(cols[i].width, ansi.StringWidth(cell))
		}
		if limit, ok := columnCaps[cols[i].key]; ok {
			cols[i].width = min(cols[i].width, limit)
		}
	}
	return layout(width, cols)
}

func (m Model) anyVisible(pred func(manager.AccountStatus) bool) bool {
	for _, idx := range m.visible {
		if pred(m.overview.Accounts[idx]) {
			return true
		}
	}
	return false
}

// adcApplies reports whether an account has application default credentials
// at all; providers without an equivalent show "–".
func adcApplies(a manager.AccountStatus) bool {
	return a.ADCPath != "" || a.StoredADCPath != "" || a.ADCExists
}

func (m Model) expiresHeader() string {
	if len(m.visible) > 0 && !m.anyVisible(func(a manager.AccountStatus) bool { return a.Budget == nil }) {
		return "BUDGET"
	}
	return "EXPIRES"
}

func renderColumnHeaders(cols []colSpec, g glyphs, st styles) string {
	cells := make([]string, len(cols))
	for i, c := range cols {
		cells[i] = fitCells(c.header, c.width, g.Ellipsis)
	}
	return strings.Repeat(" ", markerWidth) + st.Header.Render(strings.Join(cells, strings.Repeat(" ", colGap)))
}

// cellText is the unstyled text of one cell; it sizes the columns.
func (m Model) cellText(a manager.AccountStatus, key string, g glyphs) string {
	switch key {
	case colName:
		return a.Name
	case colTarget:
		return a.ProjectID
	case colIdentity:
		if a.Identity != "" {
			return a.Identity
		}
		return a.Email
	case colADC:
		switch {
		case !adcApplies(a):
			return "–"
		case a.ADCExists:
			return g.Yes
		}
		return g.No
	case colStatus:
		return statusText(a, g)
	case colExpires:
		return m.expiresText(a)
	}
	return ""
}

// renderCell styles one cell already fitted to width.
func (m Model) renderCell(a manager.AccountStatus, c colSpec, g glyphs, st styles) string {
	text := m.cellText(a, c.key, g)
	switch c.key {
	case colTarget:
		return padCells(middleEllipsis(text, c.width, g.Ellipsis), c.width)
	case colName:
		cell := fitCells(text, c.width, g.Ellipsis)
		if a.Active {
			return st.ActiveName.Render(cell)
		}
		return cell
	case colADC:
		cell := fitCells(text, c.width, g.Ellipsis)
		switch text {
		case g.Yes:
			return st.OK.Render(cell)
		case g.No:
			return st.Error.Render(cell)
		}
		return st.Dim.Render(cell)
	case colStatus:
		return healthStyle(a.Status, st).Render(fitCells(text, c.width, g.Ellipsis))
	case colExpires:
		cell := fitCells(text, c.width, g.Ellipsis)
		if a.Status == manager.HealthExpired || a.Status == manager.HealthBudgetLow {
			return healthStyle(a.Status, st).Render(cell)
		}
		return cell
	}
	if text == "" {
		return st.Dim.Render(fitCells("–", c.width, ""))
	}
	return fitCells(text, c.width, g.Ellipsis)
}

func (m Model) renderRow(a manager.AccountStatus, cols []colSpec, g glyphs, st styles, selected bool) string {
	cells := make([]string, len(cols))
	for i, c := range cols {
		cells[i] = m.renderCell(a, c, g, st)
	}
	marker := " "
	if selected {
		marker = g.Bar
	}
	dot := " "
	if a.Active {
		dot = st.OK.Render(g.Active)
	}
	line := marker + dot + strings.Join(cells, strings.Repeat(" ", colGap))
	if selected {
		line = st.Selected.Render(padCells(line, rowWidthOf(cols)+markerWidth))
	}
	return line
}

// healthGlyph is the symbol of a health state; the word follows it.
func healthGlyph(h manager.Health, g glyphs) string {
	switch h {
	case manager.HealthReady:
		return g.Active
	case manager.HealthAuthRequired, manager.HealthBudgetLow:
		return g.Warn
	case manager.HealthExpired:
		return g.No
	case manager.HealthUnverified:
		return g.Unverified
	case manager.HealthCLIMissing:
		return g.Missing
	}
	return "!"
}

func statusText(a manager.AccountStatus, g glyphs) string {
	text := healthGlyph(a.Status, g) + " " + string(a.Status)
	if a.Status == manager.HealthUnverified && a.LastGoodAge > 0 {
		text += " " + formatAge(a.LastGoodAge)
	}
	return text
}

func healthStyle(h manager.Health, st styles) lipgloss.Style {
	switch h {
	case manager.HealthReady:
		return st.OK
	case manager.HealthError, manager.HealthConfigMissing, manager.HealthExpired:
		return st.Error
	case manager.HealthADCMissing, manager.HealthAuthRequired, manager.HealthProjectMismatch, manager.HealthBudgetLow:
		return st.Warn
	case manager.HealthUnverified, manager.HealthCLIMissing:
		return st.Dim
	}
	return lipgloss.NewStyle()
}

// expiresText is the EXPIRES/BUDGET cell: the remaining budget for gateways,
// otherwise the time until (or since) the session expires.
func (m Model) expiresText(a manager.AccountStatus) string {
	if b := a.Budget; b != nil {
		symbol := "$"
		if b.Currency != "" && !strings.EqualFold(b.Currency, "USD") {
			symbol = b.Currency + " "
		}
		if b.Limit <= 0 {
			return fmt.Sprintf("%s%.2f spent", symbol, b.Spent)
		}
		return fmt.Sprintf("%s%.2f left", symbol, b.Limit-b.Spent)
	}
	if a.ExpiresAt.IsZero() {
		return "–"
	}
	left := a.ExpiresAt.Sub(m.now())
	if left <= 0 {
		return formatAge(-left) + " ago"
	}
	return formatDuration(left)
}

// formatAge is a coarse age: 14s, 5m, 3h, 2d.
func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// formatDuration keeps minutes for durations under a day: 52m, 3h 12m, 6d.
func formatDuration(d time.Duration) string {
	if d >= time.Hour && d < 24*time.Hour {
		if minutes := int(d.Minutes()) % 60; minutes > 0 {
			return fmt.Sprintf("%dh %dm", int(d.Hours()), minutes)
		}
	}
	return formatAge(d)
}
