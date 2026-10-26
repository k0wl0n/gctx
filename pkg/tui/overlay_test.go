package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestCreateFormIsAFramedModal(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 40}, {70, 24}} {
		m := resize(t, openAWS(t, formService(t)), size.width, size.height)
		m = typeText(t, m, "prod-eu-data")
		out := screen(m)
		for _, want := range []string{"┌", "┐", "└", "┘", "Create account", "step 1 of 2", "‹", "›", "writes", "never",
			"│ Create and login │", "│ Create only │", "Cancel"} {
			if !strings.Contains(out, want) {
				t.Errorf("%dx%d: create form lacks %q:\n%s", size.width, size.height, want, out)
			}
		}
		lines := strings.Split(out, "\n")
		for _, label := range []string{"Name ", "Profile name"} {
			i := lineIndex(t, out, label)
			input := lines[i]
			if i+1 < len(lines) {
				input += lines[i+1]
			}
			if !strings.Contains(input, "───") && !strings.Contains(input, "___") {
				t.Errorf("%dx%d: %q has no visible input box:\n%s", size.width, size.height, label, out)
			}
		}
	}
}

func TestCreateFormDimsTheMainScreenBehind(t *testing.T) {
	m := openForm(t, newMultiService())
	m = resize(t, m, 120, 40)
	lines := strings.Split(screen(m), "\n")
	if !strings.HasPrefix(lines[0], "gctx") || !strings.Contains(lines[2], "All ") {
		t.Errorf("header and tab rows must stay visible behind the modal:\n%s", strings.Join(lines, "\n"))
	}
	raw := m.View().Content
	st := newStylesFor(m.profile)
	if !strings.Contains(raw, st.Dim.Render(ansi.Strip(strings.Split(raw, "\n")[0]))) {
		t.Error("the main screen behind a modal must be dimmed")
	}
}

func TestOverlaysFitEverySize(t *testing.T) {
	opens := map[string]func(Model) Model{
		"create":  func(m Model) Model { return press(t, m, "c") },
		"palette": func(m Model) Model { return press(t, m, ":") },
		"delete":  func(m Model) Model { return press(t, m, "ctrl+d") },
	}
	for name, open := range opens {
		for _, width := range []int{160, 120, 90, 70, 60} {
			for _, height := range []int{12, 24, 40} {
				m := open(multiModel(t, newMultiService(), width, height))
				out := screen(m)
				lines := strings.Split(out, "\n")
				if len(lines) > height {
					t.Errorf("%s %dx%d: %d lines", name, width, height, len(lines))
				}
				for _, line := range lines {
					if w := ansi.StringWidth(line); w > width {
						t.Errorf("%s %dx%d: line of %d cells: %q", name, width, height, w, line)
					}
				}
				if !strings.Contains(out, "┌") || !strings.Contains(out, "┘") {
					t.Errorf("%s %dx%d: not framed by the shared modal:\n%s", name, width, height, out)
				}
			}
		}
	}
}

func TestDeleteModalUsesTheDestructiveBorder(t *testing.T) {
	m := press(t, multiModel(t, newMultiService(), 120, 40), "ctrl+d")
	st := newStylesFor(m.profile)
	g := newGlyphs(false)
	if !strings.Contains(m.View().Content, st.Error.Render(g.Box[0]+strings.Repeat(g.Box[4], modalBoxWidth(120, false)-2)+g.Box[1])) {
		t.Error("the delete modal must use the error colour for its border")
	}
}

func TestSelectedChipAndFocusedButtonColours(t *testing.T) {
	m := openForm(t, formService(t))
	m.profile = colorprofile.TrueColor
	m = press(t, m, "right") // AWS
	st := newStylesFor(m.profile)
	aws := m.providerInfo(provider.AWS)
	raw := m.View().Content
	if !strings.Contains(raw, filledStyle(aws.Badge.Color, st).Render(" AWS ")) {
		t.Error("the selected provider chip must be filled in the provider colour")
	}
	m = tabUntil(t, m, "[*Create only*]")
	if !strings.Contains(m.View().Content, st.TabActive.Render("  Create only  ")) {
		t.Error("the focused button must be drawn inverted")
	}
}

func TestAIProvidersShareTheFormModal(t *testing.T) {
	m := openForm(t, newMultiService())
	m = press(t, m, "up", "right") // Kind: AI tool
	out := screen(m)
	for _, want := range []string{"┌", "Kind", "Tool", "Claude Code", "Auth", "◉ subscription", "Org"} {
		if !strings.Contains(out, want) {
			t.Errorf("AI tool form lacks %q:\n%s", want, out)
		}
	}
	m = press(t, m, "right") // Kind: AI gateway
	out = screen(m)
	for _, want := range []string{"┌", "Gateway", "SumoPod", "Key from", "◉ 1Password", "Key ref", "Base URL", "writes", "never", "│ Create │"} {
		if !strings.Contains(out, want) {
			t.Errorf("AI gateway form lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Create and login") {
		t.Errorf("a provider without login offers no login button:\n%s", out)
	}
}

func TestButtonStyles(t *testing.T) {
	colour := newStylesFor(colorprofile.TrueColor)
	if got, want := renderPrimaryButton("Import 9", false, true, colour), colour.TabActive.Render(" Import 9 "); got != want {
		t.Errorf("primary button = %q, want filled %q", got, want)
	}
	secondary := renderButton("Review each", false, true, colour)
	if !strings.Contains(secondary, colour.Sep.Render("│")) || strings.Contains(ansi.Strip(secondary), "[") {
		t.Errorf("secondary button must be outlined with border-coloured bars, got %q", secondary)
	}
	if got := renderPlainButton("Start empty", false, colour); got != colour.Dim.Render("  Start empty  ") {
		t.Errorf("tertiary button must be plain dim text, got %q", got)
	}
	for _, button := range []string{secondary, renderButton("Review each", true, true, colour)} {
		if w := ansi.StringWidth(button); w != len("Review each")+4 {
			t.Errorf("buttons keep one width in every state, got %d", w)
		}
	}

	none := newStylesFor(colorprofile.ASCII)
	cases := map[string]string{
		renderPrimaryButton("Import 9", false, true, none): "[ Import 9 ]",
		renderPrimaryButton("Import 9", true, true, none):  "[*Import 9*]",
		renderButton("Review each", false, true, none):     "[ Review each ]",
		renderPlainButton("Start empty", false, none):      "  Start empty  ",
		renderPlainButton("Start empty", true, none):       "[*Start empty*]",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("no-colour button = %q, want %q", got, want)
		}
	}
}

func TestFirstRunIsABorderedPanel(t *testing.T) {
	for _, width := range []int{160, 120, 90, 70, 60} {
		m := firstRunModel(t, firstRunService(), width, 24)
		lines := strings.Split(screen(m), "\n")
		wantWidth := min(width, modalMaxWidth)
		if width < modalWideFrom {
			wantWidth = width
		}
		if !strings.HasPrefix(lines[0], strings.Repeat(" ", (width-wantWidth)/2)+"┌") {
			t.Errorf("%d cols: panel must start with a corner and be centred:\n%s", width, strings.Join(lines, "\n"))
		}
		out := strings.Join(lines, "\n")
		for _, want := range []string{"no accounts yet", "Import 11", "Review each", "Start empty", "q quit", "└"} {
			if !strings.Contains(out, want) {
				t.Errorf("%d cols: panel lacks %q:\n%s", width, want, out)
			}
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("%d cols: line is %d wide: %q", width, w, line)
			}
		}
		if len(lines) != 24 {
			t.Errorf("%d cols: %d lines, want 24", width, len(lines))
		}
	}
}

func TestFirstRunColours(t *testing.T) {
	m := firstRunModel(t, firstRunService(), 120, 30)
	m.profile = colorprofile.TrueColor
	st := newStylesFor(m.profile)
	raw := m.View().Content
	if !strings.Contains(raw, "☑") {
		t.Errorf("colour profiles draw ☑ checkboxes")
	}
	g := newGlyphs(false)
	top := st.Sep.Render(g.Box[0] + strings.Repeat(g.Box[4], modalBoxWidth(120, false)-2) + g.Box[1])
	if !strings.Contains(raw, top) {
		t.Errorf("panel border must use the border colour")
	}
	if !strings.Contains(raw, st.TabActive.Render(" Import 11 ")) && !strings.Contains(raw, st.TabActive.Underline(true).Render(" Import 11 ")) {
		t.Errorf("Import must be a filled button")
	}
	if !strings.Contains(raw, st.Dim.Render("○ TCT tccli not installed")) {
		t.Errorf("the missing CLI row must be dim")
	}
}

func TestOfflineBannerIsABoxWhenThereIsRoom(t *testing.T) {
	svc := newMultiService()
	markOffline(svc)
	m := finishChecks(checksInFlight(t, svc, 120, 40), nil)
	out := screen(m)
	if !strings.Contains(out, "┌") || !strings.Contains(out, "⚠ network unreachable · showing last known status") {
		t.Errorf("offline banner must be a box:\n%s", out)
	}
	m.profile = colorprofile.TrueColor
	if !strings.Contains(m.View().Content, newStylesFor(m.profile).Warn.Render("┌"+strings.Repeat("─", 118)+"┐")) {
		t.Error("the banner border must use the warning colour")
	}

	m = finishChecks(checksInFlight(t, svc, 70, 24), nil)
	out = screen(m)
	if strings.Contains(strings.Split(out, "\n")[3], "┌") || !strings.Contains(out, "▌ ⚠ network unreachable") || !strings.Contains(out, "NAME") {
		t.Errorf("small screens keep the one-line bar and the table:\n%s", out)
	}
}
