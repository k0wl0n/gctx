package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
)

// unreachable reports whether a provider's check could not reach its service:
// the check failed or timed out and verified no account. A check that
// verified some accounts did reach it.
func unreachable(msg credsCheckedMsg) bool {
	if msg.err != nil {
		return true
	}
	if len(msg.report.Accounts) > 0 {
		return false
	}
	for _, w := range msg.report.Warnings {
		if w.Code == manager.WarnCredentialCheck {
			return true
		}
	}
	return false
}

// offlineAfterCheck is the offline state once the latest provider reported:
// every provider unreachable means offline, one success clears it, and a
// cycle still waiting on providers keeps what was known.
func (m Model) offlineAfterCheck() bool {
	failed, pending := 0, 0
	for _, p := range m.creds.progress {
		switch {
		case !p.done:
			pending++
		case p.unreachable:
			failed++
		default:
			return false
		}
	}
	if pending == 0 {
		return failed > 0
	}
	return m.offline
}

func (m Model) offlineBanner(width int, g glyphs, st styles) string {
	return st.Warn.Render(truncateCells(g.Warn+" network unreachable "+g.Dot+" showing last known status", width, g.Ellipsis))
}

// offlineStatus is the status line while offline and no check is running.
func (m Model) offlineStatus(g glyphs, st styles) string {
	return st.Warn.Render("R retry " + g.Dot + " switching still works offline")
}

// onMissingCLITab reports whether the current tab is a provider with no
// accounts and no CLI: there is nothing to list, only an explanation.
func (m Model) onMissingCLITab() bool {
	info, ok := m.tabProvider()
	return ok && !info.CLI.Found && info.Count == 0
}

func cliLabel(info manager.ProviderInfo) string {
	if info.CLI.Name != "" {
		return info.CLI.Name
	}
	return info.Label + " CLI"
}

// missingCLILines is the centred explanation of a tab whose CLI is missing,
// exactly height lines.
func (m Model) missingCLILines(width, height int, g glyphs, st styles) []string {
	info, _ := m.tabProvider()
	center := func(text string) string {
		return strings.Repeat(" ", max((width-ansi.StringWidth(text))/2, 0)) + text
	}
	cli := cliLabel(info)
	out := []string{"", "", center(st.Dim.Render(g.Missing)), "", center(st.Bold.Render(cli + " is not installed")), ""}
	explain := info.Label + " accounts need it for login, switching and auth checks. Other clouds are unaffected."
	for _, line := range wrapText(explain, max(min(width-4, 100), 20)) {
		out = append(out, center(st.Dim.Render(line)))
	}
	out = append(out, "")

	command := installCommand(info.InstallHint)
	switch {
	case command == "":
		command = "install " + cli + " and put it on PATH"
	case !strings.Contains(command, "://"):
		command = "$ " + command
	}
	inner := []string{command, "then press R to rescan PATH"}
	boxWidth := 0
	for _, line := range inner {
		boxWidth = max(boxWidth, ansi.StringWidth(line))
	}
	boxWidth = min(boxWidth, max(width-6, 10))
	horizontal := strings.Repeat(g.Box[4], boxWidth+2)
	out = append(out, center(st.Sep.Render(g.Box[0]+horizontal+g.Box[1])))
	for i, line := range inner {
		cell := fitCells(line, boxWidth, g.Ellipsis)
		if i == 1 {
			cell = st.Dim.Render(cell)
		}
		out = append(out, center(st.Sep.Render(g.Box[5])+" "+cell+" "+st.Sep.Render(g.Box[5])))
	}
	out = append(out, center(st.Sep.Render(g.Box[2]+horizontal+g.Box[3])))

	if len(out) > height {
		out = out[:height]
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// copyInstallCommand copies the install command of the missing-CLI tab.
func (m Model) copyInstallCommand() (Model, tea.Cmd) {
	info, _ := m.tabProvider()
	command := installCommand(info.InstallHint)
	if command == "" {
		return m, m.setStatus("No install command is known for "+cliLabel(info), levelInfo)
	}
	return m, tea.Batch(tea.SetClipboard(command), m.setStatus("copied: "+command, levelInfo))
}

// missingCLIHints is the hint line of the missing-CLI tab.
func missingCLIHints(st styles) string {
	parts := []string{"y " + st.Dim.Render("copy"), "R " + st.Dim.Render("rescan"), "tab " + st.Dim.Render("next provider")}
	return strings.Join(parts, "  ")
}
