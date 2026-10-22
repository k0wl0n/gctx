package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// paneWidth is the detail pane's share of a wide terminal.
func paneWidth(width int) int { return max(44, width*28/100) }

// paneLines is the detail pane for the selected account: badge, name,
// active state, the provider's Details() rows, the env activation applies
// and the account's actions.
func (m Model) paneLines(width int, g glyphs, st styles) []string {
	if group, ok := m.selectedPlaceholder(); ok {
		return []string{
			truncateCells(renderBadge(group.info.Badge, st.prof, g)+" "+st.Bold.Render(strings.ToUpper(group.info.Label)), width, g.Ellipsis),
			"",
			truncateCells(m.groupSummary(group.info, g), width, g.Ellipsis),
			st.Dim.Render(truncateCells(fmt.Sprintf("folded: %d ready rows hidden", group.hidden), width, g.Ellipsis)),
			"",
			keys.Fold.Keys()[0] + " " + st.Dim.Render("expand"),
		}
	}
	acct, ok := m.selectedAccount()
	if !ok {
		return []string{st.Dim.Render("No account selected")}
	}
	info := m.providerInfo(acct.Provider)
	state := st.Dim.Render(g.Missing + " not active")
	if acct.Active {
		state = tint(info.Badge.Color, st.prof, g.Active) + " active"
	}
	var out []string
	add := func(line string) { out = append(out, truncateCells(line, width, g.Ellipsis)) }
	row := func(label, value string) {
		add(st.Dim.Render(fitCells(strings.ToLower(label), 14, g.Ellipsis)) + " " + value)
	}

	add(joinSides(renderBadge(info.Badge, st.prof, g)+" "+st.Bold.Render(acct.Name), state, width, g))
	add("")
	if acct.ProjectID != "" && !slices.ContainsFunc(acct.Details, func(f provider.Field) bool { return f.Value == acct.ProjectID }) {
		row(info.TargetLabel, acct.ProjectID)
	}
	for _, field := range acct.Details {
		if field.Value != "" {
			row(field.Label, field.Value)
		}
	}
	row("status", healthStyle(acct.Status, st).Render(statusText(acct, g)))
	if identity := m.cellText(acct, colIdentity, g); identity != "" {
		row("identity", middleEllipsis(identity, width-15, g.Ellipsis))
	}
	if text := m.expiresText(acct); text != "–" {
		label := "expires"
		if acct.Budget != nil {
			label = "budget"
		}
		row(label, text)
	}
	if !acct.Checked.IsZero() {
		row("checked", formatAge(max(m.now().Sub(acct.Checked), 0))+" ago")
	}

	add("")
	add(st.Dim.Render("env applied on activate"))
	env := m.envs[acct.Name]
	if len(env) == 0 {
		add(st.Dim.Render("–"))
	}
	for _, kv := range env {
		add(displayEnv(kv))
	}
	add("")
	add(st.Dim.Render("apply in this shell"))
	add(fmt.Sprintf(`$ eval "$(%s env %s)"`, config.AppName, acct.Name))
	add("")
	line := ""
	for _, a := range []struct{ key, label string }{
		{g.Enter, "activate"}, {keys.Login.Keys()[0], "login"}, {keys.Shell.Keys()[0], "shell"},
		{keys.Run.Keys()[0], "run"}, {keys.Delete.Keys()[0], "delete"},
	} {
		action := a.key + " " + st.Dim.Render(a.label)
		if line != "" && ansi.StringWidth(line)+2+ansi.StringWidth(action) > width {
			add(line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += action
	}
	add(line)
	return out
}
