package tui

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

const (
	helpColumnGap  = 3
	helpSplitWidth = 100 // from this width the key groups sit side by side
)

type cliVersionsMsg struct{ versions map[provider.ID]string }

// openHelp shows the help screen. The CLI versions are probed once, in the
// background, so opening help never waits for a subprocess.
func (m Model) openHelp() (Model, tea.Cmd) {
	m.screen = screenHelp
	m.helpOffset = 0
	if m.versionsLoaded || m.versionsLoading {
		return m, nil
	}
	m.versionsLoading = true
	svc := m.svc
	return m, func() tea.Msg { return cliVersionsMsg{versions: svc.CLIVersions(context.Background())} }
}

func (m Model) handleCLIVersions(msg cliVersionsMsg) (Model, tea.Cmd) {
	m.cliVersions = msg.versions
	m.versionsLoaded, m.versionsLoading = true, false
	return m, nil
}

// updateHelp handles keys on the help screen: close, or scroll when the
// content is taller than the terminal.
func (m Model) updateHelp(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	page := max(m.helpBodyHeight()-1, 1)
	switch msg.String() {
	case "esc", "q", "?":
		m.screen = screenMain
	case "j", "down":
		m.helpOffset++
	case "k", "up":
		m.helpOffset--
	case "ctrl+f", "pgdown":
		m.helpOffset += page
	case "ctrl+b", "pgup":
		m.helpOffset -= page
	case "g", "home":
		m.helpOffset = 0
	case "G", "end":
		m.helpOffset = len(m.helpBody(m.width))
	}
	m.helpOffset = max(min(m.helpOffset, m.helpMaxOffset()), 0)
	return m, nil
}

// helpBodyHeight is the rows between the title rule and the footer rule.
func (m Model) helpBodyHeight() int { return max(m.height-4, 1) }

func (m Model) helpMaxOffset() int {
	return max(len(m.helpBody(m.width))-m.helpBodyHeight(), 0)
}

func (m Model) viewHelp() string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	body := m.helpBody(m.width)
	height := m.helpBodyHeight()
	offset := max(min(m.helpOffset, m.helpMaxOffset()), 0)
	shown := body[offset:min(offset+height, len(body))]

	closeHint := "esc or ? to close"
	if len(body) > height {
		closeHint = "j/k scroll · " + closeHint
	}
	title := st.Title.Render("Keys")
	if len(body) > height && offset < m.helpMaxOffset() {
		title += st.Dim.Render("  " + g.Dot + " more below")
	}
	out := []string{
		joinSides(title, st.Dim.Render(closeHint), m.width, g),
		hRule(g, st, m.width),
	}
	for i := range height {
		line := ""
		if i < len(shown) {
			line = truncateCells(shown[i], m.width, g.Ellipsis)
		}
		out = append(out, line)
	}
	out = append(out, hRule(g, st, m.width), st.Dim.Render(truncateCells(m.helpFooter(), m.width, g.Ellipsis)))
	return strings.Join(out, "\n")
}

// helpFooter names the files the session reads and writes.
func (m Model) helpFooter() string {
	text := "config " + tildePath(m.overview.ConfigPath)
	if m.logPath != "" {
		text += " · log " + tildePath(m.logPath)
	}
	return text
}

func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return path
}

// helpBody is every body row of the help screen: the key groups in as many
// columns as fit, then the providers block.
func (m Model) helpBody(width int) []string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	body := helpKeyColumns(keys.helpColumns(), width-4, g, st)
	body = append(body, "", hRule(g, st, width-4), "")
	body = append(body, m.helpProviders(width-4, g, st)...)
	for i := range body {
		body[i] = "  " + body[i]
	}
	return body
}

// helpBlock is one group as heading and aligned rows, every row padded to width.
type helpBlock struct {
	lines []string
	width int
}

func newHelpBlock(group keyGroup, st styles) helpBlock {
	keyWidth, descWidth := 0, 0
	for _, binding := range group.Bindings {
		h := binding.Help()
		keyWidth = max(keyWidth, ansi.StringWidth(h.Key))
		descWidth = max(descWidth, ansi.StringWidth(h.Desc))
	}
	block := helpBlock{width: max(keyWidth+2+descWidth, ansi.StringWidth(group.Title))}
	block.lines = append(block.lines, st.Dim.Render(strings.ToLower(group.Title)))
	for _, binding := range group.Bindings {
		h := binding.Help()
		block.lines = append(block.lines, st.Bold.Render(padCells(h.Key, keyWidth))+"  "+h.Desc)
	}
	return block
}

// helpKeyColumns lays the groups out side by side from helpSplitWidth, each
// column as wide as its content. When three columns do not fit, the last two
// groups share one column; narrower screens stack them.
func helpKeyColumns(groups []keyGroup, width int, g glyphs, st styles) []string {
	blocks := make([]helpBlock, len(groups))
	for i, group := range groups {
		blocks[i] = newHelpBlock(group, st)
	}
	if width >= helpSplitWidth {
		if columns := fitColumns(blocks, width); columns != nil {
			return joinColumns(columns, columnWidths(columns), g)
		}
		if len(blocks) > 2 {
			merged := append(slices.Clone(blocks[1].lines), "")
			merged = append(merged, blocks[2].lines...)
			columns := [][]string{blocks[0].lines, merged}
			if widths := columnWidths(columns); sumWidths(widths) <= width {
				return joinColumns(columns, widths, g)
			}
		}
	}
	var out []string
	for i, block := range blocks {
		if i > 0 {
			out = append(out, "")
		}
		for _, line := range block.lines {
			out = append(out, truncateCells(line, width, g.Ellipsis))
		}
	}
	return out
}

func fitColumns(blocks []helpBlock, width int) [][]string {
	columns := make([][]string, len(blocks))
	for i, block := range blocks {
		columns[i] = block.lines
	}
	if sumWidths(columnWidths(columns)) > width {
		return nil
	}
	return columns
}

func columnWidths(columns [][]string) []int {
	widths := make([]int, len(columns))
	for i, column := range columns {
		for _, line := range column {
			widths[i] = max(widths[i], ansi.StringWidth(line))
		}
	}
	return widths
}

func sumWidths(widths []int) int {
	total := helpColumnGap * (len(widths) - 1)
	for _, w := range widths {
		total += w
	}
	return total
}

func joinColumns(columns [][]string, widths []int, g glyphs) []string {
	rows := 0
	for _, column := range columns {
		rows = max(rows, len(column))
	}
	out := make([]string, rows)
	for row := range rows {
		var line strings.Builder
		for i, column := range columns {
			cell := ""
			if row < len(column) {
				cell = column[row]
			}
			if i < len(columns)-1 {
				cell = padCells(truncateCells(cell, widths[i], g.Ellipsis), widths[i]) + strings.Repeat(" ", helpColumnGap)
			}
			line.WriteString(cell)
		}
		out[row] = strings.TrimRight(line.String(), " ")
	}
	return out
}

var versionPattern = regexp.MustCompile(`\d+(?:\.\d+)+`)

// shortVersion picks the version number out of a `--version` first line.
func shortVersion(line string) string {
	if version := versionPattern.FindString(line); version != "" {
		return version
	}
	return line
}

// installAdvice is the hint without its "install the X CLI:" lead-in.
func installAdvice(hint string) string {
	if _, after, ok := strings.Cut(hint, ": "); ok {
		return after
	}
	return hint
}

// installCommand is the first command of an install hint, or its first URL.
func installCommand(hint string) string {
	command := installAdvice(hint)
	for _, separator := range []string{", or ", " or ", " · ", ". "} {
		if before, _, ok := strings.Cut(command, separator); ok {
			command = before
		}
	}
	return strings.TrimSpace(command)
}

// helpProviders is the providers block: label, CLI and version, capabilities.
func (m Model) helpProviders(width int, g glyphs, st styles) []string {
	infos := slices.Clone(m.overview.Providers)
	slices.SortStableFunc(infos, func(a, b manager.ProviderInfo) int { return categoryRank(a.Category) - categoryRank(b.Category) })
	out := []string{st.Dim.Render("providers")}
	if len(infos) == 0 {
		return append(out, st.Dim.Render("none registered"))
	}
	labelWidth, cliWidth := 0, 0
	cells := make([]string, len(infos))
	for i, info := range infos {
		labelWidth = max(labelWidth, ansi.StringWidth(info.Label))
		cells[i] = m.cliCell(info)
		cliWidth = max(cliWidth, ansi.StringWidth(cells[i]))
	}
	cliWidth = min(cliWidth, max(width-labelWidth-24, 16))
	for i, info := range infos {
		label := tint(info.Badge.Color, st.prof, st.Bold.Render(padCells(info.Label, labelWidth)))
		mark, detail := st.OK.Render(g.Yes), m.capabilities(info)
		if !info.CLI.Found {
			mark, detail = st.Dim.Render(g.Missing), installAdvice(info.InstallHint)
			if detail == "" {
				detail = "install " + info.CLI.Name + " and put it on PATH"
			}
		}
		cell := fitCells(cells[i], cliWidth, g.Ellipsis)
		if !info.CLI.Found {
			cell = st.Dim.Render(cell)
		}
		out = append(out, truncateCells(label+"  "+cell+"  "+mark+" "+detail, width, g.Ellipsis))
	}
	return out
}

// cliCell is "<cli> <version>", the version being … until it is loaded.
func (m Model) cliCell(info manager.ProviderInfo) string {
	name := info.CLI.Name
	switch {
	case name == "" && info.CLI.Found:
		return "no CLI needed"
	case name == "":
		name = info.Label + " CLI"
	}
	if !info.CLI.Found {
		return name + " not found"
	}
	switch version := m.cliVersions[info.ID]; {
	case !m.versionsLoaded:
		return name + " …"
	case version == "":
		return name
	default:
		return name + " " + shortVersion(version)
	}
}

// capabilities describes how the provider switches, from its metadata and
// the env variable names its accounts export (never their values).
func (m Model) capabilities(info manager.ProviderInfo) string {
	var parts []string
	if info.EnvBased {
		names := m.envNames(info.ID)
		if len(names) == 0 {
			parts = append(parts, "env-based")
		} else {
			parts = append(parts, "env: "+strings.Join(names, ", "))
		}
	} else {
		parts = append(parts, "global switch")
	}
	for _, account := range m.overview.Accounts {
		if account.Provider == info.ID && adcApplies(account) {
			parts = append(parts, "ADC")
			break
		}
	}
	if !info.CanLogin {
		parts = append(parts, "no login")
	}
	return strings.Join(parts, " · ")
}

// envNames lists the variable names the provider's first account with an env exports.
func (m Model) envNames(id provider.ID) []string {
	for _, account := range m.overview.Accounts {
		if account.Provider != id {
			continue
		}
		var names []string
		for _, kv := range m.envs[account.Name] {
			if name, _, ok := strings.Cut(kv, "="); ok {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	return nil
}
