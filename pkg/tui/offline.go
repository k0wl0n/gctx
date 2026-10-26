package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
)

// unreachable reports whether a network provider's check reached nothing:
// the manager lists it in NetworkFailed (every account failed with a non-auth
// error), the check itself failed, or it was cut short with no account verified.
func unreachable(msg credsCheckedMsg) bool {
	if msg.err != nil || slices.Contains(msg.report.NetworkFailed, msg.provider) {
		return true
	}
	if len(msg.report.Accounts) > 0 {
		return false
	}
	return slices.ContainsFunc(msg.report.Warnings, func(w manager.Warning) bool { return w.Code == manager.WarnCredentialCheck })
}

// offlineAfterCheck is the offline state once the latest provider reported.
// Only network-checked providers count: when every one that ran is
// unreachable the network is down, one success clears it, and a cycle still
// waiting on providers keeps what was known.
func (m Model) offlineAfterCheck() bool {
	failed, pending := 0, 0
	for _, p := range m.creds.progress {
		switch {
		case !p.network:
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
	return m.offline && failed+pending > 0
}

// offlineBannerBoxMinHeight is the screen height from which the banner is an
// amber box (three rows); shorter screens keep the one-line bar so the table
// is not pushed off.
const offlineBannerBoxMinHeight = 30

func (m Model) offlineBanner(width int, g glyphs, st styles) []string {
	text := g.Warn + " network unreachable " + g.Dot + " showing last known status"
	if m.height < offlineBannerBoxMinHeight {
		// An amber bar marks the banner as a notice, like the modal borders.
		return []string{st.Warn.Render(truncateCells(g.Bar+" "+text, width, g.Ellipsis))}
	}
	inner := max(width-4, 10)
	horizontal := strings.Repeat(g.Box[4], width-2)
	out := []string{st.Warn.Render(g.Box[0] + horizontal + g.Box[1])}
	for _, line := range wrapText(text, inner) {
		out = append(out, st.Warn.Render(g.Box[5]+" "+fitCells(line, inner, g.Ellipsis)+" "+g.Box[5]))
	}
	return append(out, st.Warn.Render(g.Box[2]+horizontal+g.Box[3]))
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
	g := newGlyphs(asciiRequested())
	parts := []string{
		keys.CopyEnv.Help().Key + " " + st.Dim.Render("copy"),
		keys.Reload.Help().Key + " " + st.Dim.Render("rescan"),
		keys.NextTab.Help().Key + " " + st.Dim.Render("next provider"),
	}
	return strings.Join(parts, st.Dim.Render(" "+g.Dot+" "))
}
