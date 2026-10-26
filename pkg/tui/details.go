package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const detailLabelWidth = 18

// sectionRule is "── title ────" across width.
func sectionRule(title string, width int, g glyphs, st styles) string {
	head := g.HRule + g.HRule + " "
	fill := width - ansi.StringWidth(head) - ansi.StringWidth(title) - 1
	return st.Sep.Render(head) + st.Dim.Render(title) + " " + st.Sep.Render(strings.Repeat(g.HRule, max(fill, 0)))
}

// renderDetails is the full-screen details of the selected account:
// breadcrumb header, ruled sections with dim labels, then the key hints.
func renderDetails(m Model) string {
	g := newGlyphs(asciiRequested())
	st := newStylesFor(m.profile)
	width := max(m.width, minWidth)
	acct, ok := m.selectedAccount()
	if !ok {
		return st.Dim.Render("No account selected")
	}
	info := m.providerInfo(acct.Provider)

	state := st.Dim.Render(g.Missing + " not active")
	if acct.Active {
		state = tint(info.Badge.Color, st.prof, g.Active) + " active"
	}
	crumb := st.Dim.Render("accounts "+detailCrumbSep(g)) + " " +
		renderBadge(info.Badge, st.prof, g) + " " + st.Bold.Render(acct.Name) + " " + st.Dim.Render(detailCrumbSep(g)+" details")
	out := []string{joinSides(crumb, state, width, g), hRule(g, st, width)}

	row := func(label, value string) {
		out = append(out, truncateCells("  "+st.Dim.Render(fitCells(label, detailLabelWidth, g.Ellipsis))+" "+value, width, g.Ellipsis))
	}
	section := func(title string) { out = append(out, "", sectionRule(title, width, g, st)) }
	yesNo := func(v bool) string {
		if v {
			return st.OK.Render(g.Yes + " yes")
		}
		return st.Warn.Render(g.No + " no")
	}
	dash := func(v string) string {
		if v == "" {
			return st.Dim.Render("–")
		}
		return middleEllipsis(v, width-detailLabelWidth-4, g.Ellipsis)
	}

	section("account")
	if label := info.TargetLabel; label != "" || acct.ProjectID != "" {
		row(strings.ToLower(orDefault(label, "target")), dash(acct.ProjectID))
	}
	row("email", dash(acct.Email))
	row("status", healthStyle(acct.Status, st).Render(statusText(acct, g)))
	if !acct.CreatedAt.IsZero() {
		row("created", acct.CreatedAt.Format(time.RFC3339))
	}

	section("findings")
	if len(acct.Findings) == 0 {
		out = append(out, "  "+st.OK.Render(g.Yes)+" "+st.Dim.Render("no issues found"))
	}
	for _, finding := range acct.Findings {
		line := healthStyle(finding.Health, st).Render(g.Warn+" "+string(finding.Health)) + " " + st.Dim.Render(g.HRule+" "+findingRemedy(finding))
		out = append(out, truncateCells("  "+line, width, g.Ellipsis))
	}

	if acct.ConfigName != "" {
		section("gcloud configuration")
		row("configuration", acct.ConfigName+" "+st.Dim.Render("exists: ")+yesNo(acct.GcloudConfigExists))
		row("account", dash(acct.GcloudAccount))
		row("project", dash(acct.GcloudProject))
	}

	adcPath := acct.StoredADCPath
	if adcPath == "" {
		adcPath = acct.ADCPath
	}
	if adcPath != "" || acct.ADCExists {
		section("credentials")
		row("stored adc path", dash(adcPath))
		row("adc exists", yesNo(acct.ADCExists))
		if acct.ADCExists && !acct.ADCModTime.IsZero() {
			row("adc modified", acct.ADCModTime.Format(time.RFC3339))
		}
	}

	if len(acct.Details) > 0 {
		section("provider details")
		for _, field := range acct.Details {
			row(strings.ToLower(field.Label), dash(field.Value))
		}
	}
	if env := m.envs[acct.Name]; len(env) > 0 {
		section("env applied on activate")
		for _, kv := range env {
			out = append(out, truncateCells("  "+displayEnv(kv), width, g.Ellipsis))
		}
	}

	footer := keyChip("esc", st) + " " + st.Dim.Render("close") + "  " + keyChip("d", st) + " " + st.Dim.Render("toggle")
	body := max(m.height-2, 3)
	if len(out) > body {
		out = out[:body]
	}
	for len(out) < body {
		out = append(out, "")
	}
	return strings.Join(append(out, hRule(g, st, width), footer), "\n")
}

func detailCrumbSep(g glyphs) string {
	if g.Caret == ">" {
		return ">"
	}
	return "›"
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
