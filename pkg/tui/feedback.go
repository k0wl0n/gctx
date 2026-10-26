package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
)

// feedbackPanel is shown after activating an env-based account: the active
// account changed in gctx, but the terminal it runs in did not.
type feedbackPanel struct {
	providerLabel string
	badgeColor    string
	account       string
	replaced      string
	evalLine      string
	slot          string
}

// panelFits reports whether the panel can be drawn on this terminal: the
// header, tabs, column headers, status and hints take up to 8 rows. The view
// and the switch status share this so the user always gets one confirmation.
func (m Model) panelFits() bool {
	return m.height-8-feedbackHeight >= minListRows
}

// feedbackStatus is the one-line confirmation used when the panel cannot be drawn.
func feedbackStatus(f *feedbackPanel) string {
	return f.account + " active for " + f.slot + " · terminal unchanged: " + f.evalLine
}

// feedbackHeight is the panel's maximum height; it is only drawn when the
// list keeps at least minListRows rows.
const (
	feedbackHeight = 6
	minListRows    = 3
)

// envEvalLine is the lookup command that applies an account's env to a shell.
func envEvalLine(account string) string {
	return fmt.Sprintf(`eval "$(%s env %s)"`, config.AppName, account)
}

func (m Model) newFeedback(res manager.SwitchResult) *feedbackPanel {
	panel := &feedbackPanel{account: res.Account, replaced: res.Replaced, evalLine: res.ApplyHint, slot: res.Slot}
	if account, ok := m.overview.Account(res.Account); ok {
		info := m.providerInfo(account.Provider)
		panel.providerLabel, panel.badgeColor = info.Label, info.Badge.Color
	}
	return panel
}

// feedbackLines draws the panel at width, nil when none is open.
func (m Model) feedbackLines(width int, g glyphs, st styles) []string {
	f := m.feedback
	if f == nil {
		return nil
	}
	inner := width - 4
	first := st.OK.Render(g.Yes) + " " + f.providerLabel + " active account is now " + st.Bold.Render(f.account)
	if f.replaced != "" {
		first += st.Dim.Render(" (was " + f.replaced + ")")
	}
	body := []string{
		joinSides(first, st.Dim.Render(keys.Clear.Keys()[0]+" dismiss"), inner, g),
		st.Dim.Render(truncateCells(f.providerLabel+" is env-based: this terminal is unchanged until you apply it.", inner, g.Ellipsis)),
	}
	command := "$ " + f.evalLine
	actions := keyChip(keys.CopyEnv.Keys()[0], st) + " " + st.Dim.Render("copy that line") +
		"  " + keyChip(keys.Shell.Keys()[0], st) + " " + st.Dim.Render("open subshell") +
		"  " + keyChip(keys.Run.Keys()[0], st) + " " + st.Dim.Render("run here")
	if ansi.StringWidth(command)+3+ansi.StringWidth(actions) <= inner {
		body = append(body, command+"   "+actions)
	} else {
		body = append(body, truncateCells(command, inner, g.Ellipsis), actions)
	}

	border := lipgloss.NormalBorder()
	if asciiRequested() {
		border = lipgloss.ASCIIBorder()
	}
	style := lipgloss.NewStyle().Border(border).Padding(0, 1).Width(width)
	if col := badgeColor(f.badgeColor, st.prof); col != nil {
		style = style.BorderForeground(col)
	}
	return strings.Split(style.Render(strings.Join(body, "\n")), "\n")
}
