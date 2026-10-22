package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
)

func openPalette(t *testing.T, svc *fakeService, selected string, width, height int) (Model, *palette) {
	t.Helper()
	m := multiModel(t, svc, width, height)
	if selected != "" {
		m.selected = selected
	}
	m = press(t, m, ":")
	p, ok := m.modal.(*palette)
	if !ok {
		t.Fatalf("':' opened %T", m.modal)
	}
	return m, p
}

func titles(entries []paletteEntry) []string {
	var out []string
	for _, entry := range entries {
		out = append(out, entry.Title)
	}
	return out
}

func entryNamed(t *testing.T, entries []paletteEntry, title string) paletteEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Title == title {
			return entry
		}
	}
	t.Fatalf("no entry %q in %v", title, titles(entries))
	return paletteEntry{}
}

func TestPaletteListsActionsAndAccounts(t *testing.T) {
	m, p := openPalette(t, newMultiService(), "cloudsuite", 150, 50)
	for _, want := range []string{
		"Login all stale accounts", "Recheck credentials", "Show help", "Toggle detail pane", "Fold or unfold provider group",
		"Create account", "Quit", "Activate cloudsuite", "Login cloudsuite", "Delete cloudsuite", "Save current ADC to cloudsuite",
		"Edit cloudsuite", "Show details for cloudsuite",
	} {
		entryNamed(t, p.entries, want)
	}
	jumps := 0
	for _, entry := range p.entries {
		if strings.HasPrefix(entry.CLI, "jump") {
			jumps++
		}
	}
	if jumps != len(m.overview.Accounts) {
		t.Errorf("%d jump entries, want one per account (%d)", jumps, len(m.overview.Accounts))
	}
	out := screen(m)
	for _, want := range []string{"actions", "accounts", "matches", "↑↓ select", "↵ run", "⇥ complete", "esc close", "right column = the CLI equivalent"} {
		if !strings.Contains(out, want) {
			t.Errorf("palette lacks %q:\n%s", want, out)
		}
	}
	m = press(t, m, "l", "e", "g", "a", "c", "y")
	out = screen(m)
	if line := lineWith(t, out, "legacy-aws"); !strings.Contains(line, "⚠") {
		t.Errorf("legacy-aws (AUTH REQUIRED) has no warning marker: %q", line)
	}
	m = press(t, m, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "b", "y", "o", "i", "p")
	if line := lineWith(t, screen(m), "byoip-lab"); !strings.Contains(line, "✗") {
		t.Errorf("byoip-lab (EXPIRED) has no failure marker: %q", line)
	}
}

func TestPaletteFuzzyOrder(t *testing.T) {
	entries := []paletteEntry{
		{Title: "Show help"}, {Title: "Create account (blog)"}, {Title: "Login all stale accounts"}, {Title: "Toggle detail pane"},
	}
	ranked := rankEntries("log", entries)
	got := titles(ranked)
	if len(got) < 2 || got[0] != "Login all stale accounts" || got[1] != "Create account (blog)" {
		t.Fatalf("ranking %v", got)
	}
	if slices.Contains(got, "Toggle detail pane") {
		t.Error("a non-subsequence matched")
	}
	if idx := slices.Index(got, "Show help"); idx >= 0 && idx < 1 {
		t.Errorf("Show help outranks Login all stale accounts: %v", got)
	}
	if _, _, ok := fuzzyMatch("LOG", "login"); !ok {
		t.Error("matching must ignore case")
	}
	if _, positions, _ := fuzzyMatch("lgn", "login"); !slices.Equal(positions, []int{0, 2, 4}) {
		t.Errorf("positions %v", positions)
	}
}

func TestPaletteShowsCLIEquivalent(t *testing.T) {
	m, p := openPalette(t, newMultiService(), "prod-aws", 160, 60)
	app := config.AppName
	for title, want := range map[string]string{
		"Login prod-aws":              app + " login prod-aws",
		"Activate prod-aws":           app + " switch prod-aws",
		"Delete prod-aws":             app + " delete prod-aws",
		"Subshell with only prod-aws": app + " shell prod-aws",
		"Copy env line for prod-aws":  app + " env prod-aws",
		"Show details for prod-aws":   app + " info prod-aws",
		"Create account":              app + " create ...",
	} {
		if got := entryNamed(t, p.entries, title).CLI; got != want {
			t.Errorf("%s: CLI %q, want %q", title, got, want)
		}
	}
	for _, title := range []string{"Login all stale accounts", "Edit prod-aws", "Show help", "Quit"} {
		if got := entryNamed(t, p.entries, title).CLI; got != "" {
			t.Errorf("%s has CLI %q but no such command exists", title, got)
		}
	}
	if cli := entryNamed(t, p.entries, "Run command as prod-aws").CLI; cli != app+" run prod-aws -- ..." {
		t.Errorf("run CLI %q", cli)
	}
	for _, title := range titles(p.entries) {
		if strings.Contains(title, "Save current ADC") {
			t.Error("save offered for an account without ADC")
		}
	}
	out := screen(m)
	if line := lineWith(t, out, "Login all stale accounts"); !strings.Contains(line, "(tui)") {
		t.Errorf("no (tui) marker: %q", line)
	}
	for _, invented := range []string{"logout", "doctor", "--stale"} {
		if strings.Contains(out, invented) {
			t.Errorf("palette shows invented command %q", invented)
		}
	}
}

func TestPaletteRunsAction(t *testing.T) {
	svc := newMultiService()
	m, _ := openPalette(t, svc, "cloudsuite", 150, 50)
	m = press(t, m, "d", "e", "l", "e", "t", "e", "enter")
	dlg, ok := m.modal.(*deleteDialog)
	if !ok || dlg.account.Name != "cloudsuite" {
		t.Fatalf("running Delete opened %T", m.modal)
	}

	m, _ = openPalette(t, svc, "cloudsuite", 150, 50)
	m = press(t, m, "s", "h", "o", "w", " ", "h", "enter")
	if m.modal != nil || m.screen != screenHelp {
		t.Errorf("Show help: modal %T screen %v", m.modal, m.screen)
	}
}

func TestPaletteJumpSelectsAccount(t *testing.T) {
	m, _ := openPalette(t, newMultiService(), "cloudsuite", 150, 50)
	m = press(t, m, "o", "p", "e", "n", "r", "enter")
	if m.modal != nil || m.selected != "openrouter" {
		t.Errorf("jump: modal %T selected %q", m.modal, m.selected)
	}
}

func TestPaletteEscCloses(t *testing.T) {
	m, _ := openPalette(t, newMultiService(), "cloudsuite", 150, 50)
	m = press(t, m, "x", "esc")
	if m.modal != nil {
		t.Error("esc must close the palette")
	}
}

func TestPaletteTabCompletes(t *testing.T) {
	m, p := openPalette(t, newMultiService(), "cloudsuite", 150, 50)
	m = press(t, m, "l", "o", "g", "i", "n", " ", "a", "tab")
	if got := m.modal.(*palette).input.Value(); got != "Login all stale accounts" {
		t.Errorf("tab completed to %q", got)
	}
	_ = p
}

func TestEveryKeyBindingHasPaletteEntry(t *testing.T) {
	// Applicability depends on the row: ADC saving needs a GCP row, the env
	// line an env-based one. Together the two selections offer every action.
	_, gcpRow := openPalette(t, newMultiService(), "cloudsuite", 150, 50)
	_, envRow := openPalette(t, newMultiService(), "prod-aws", 150, 50)
	p := &palette{entries: append(slices.Clone(gcpRow.entries), envRow.entries...)}
	for _, group := range keys.groups() {
		for _, binding := range group.Bindings {
			first := binding.Keys()[0]
			if reason, exempt := paletteExempt[first]; exempt {
				if reason == "" {
					t.Errorf("exemption for %q has no reason", first)
				}
				continue
			}
			found := slices.ContainsFunc(p.entries, func(entry paletteEntry) bool { return entry.Key == first })
			if !found {
				t.Errorf("binding %q (%s) has no palette entry and no exemption: new features add palette entries first (D16)", first, binding.Help().Desc)
			}
		}
	}
	for first := range paletteExempt {
		if slices.ContainsFunc(p.entries, func(entry paletteEntry) bool { return entry.Key == first }) {
			t.Errorf("%q is exempt but has an entry", first)
		}
	}
}

func TestPaletteDisablesRowActionsWithoutSelection(t *testing.T) {
	svc := newFakeService(sampleOverview(""))
	m := newTestModel(t, svc, 120, 40)
	m = press(t, m, ":")
	p := m.modal.(*palette)
	for _, title := range []string{"Login selected account", "Delete selected account", "Activate selected account"} {
		if entry := entryNamed(t, p.entries, title); !entry.disabled {
			t.Errorf("%s must be disabled with nothing selected", title)
		}
	}
	for _, title := range []string{"Create account", "Show help", "Quit"} {
		if entry := entryNamed(t, p.entries, title); entry.disabled {
			t.Errorf("%s must stay enabled", title)
		}
	}
	m = press(t, m, "D", "e", "l", "e", "t", "e")
	m, _ = m.dispatch(keyMsg("enter")) // dispatch: press would wait for the status timer
	if _, ok := m.modal.(*deleteDialog); ok {
		t.Error("a disabled entry ran")
	}
}

func TestPaletteLayoutAt120And70(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 40}, {70, 30}} {
		m, _ := openPalette(t, newMultiService(), "cloudsuite", size.width, size.height)
		out := screen(m)
		lines := strings.Split(out, "\n")
		if len(lines) > size.height {
			t.Errorf("%dx%d: %d lines", size.width, size.height, len(lines))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > size.width {
				t.Errorf("%dx%d: line is %d wide: %q", size.width, size.height, w, line)
			}
		}
		for _, want := range []string{":", "matches", "actions", "esc close"} {
			if !strings.Contains(out, want) {
				t.Errorf("%dx%d lacks %q:\n%s", size.width, size.height, want, out)
			}
		}
	}
}
