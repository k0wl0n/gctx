package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func helpScreenAt(t *testing.T, width, height int) string {
	t.Helper()
	return screen(press(t, multiModel(t, withTencent(newMultiService()), width, height), "?"))
}

func TestHelpColumnsCoverEveryBinding(t *testing.T) {
	var fromGroups, fromColumns []string
	for _, group := range keys.groups() {
		for _, binding := range group.Bindings {
			fromGroups = append(fromGroups, binding.Help().Key+" "+binding.Help().Desc)
		}
	}
	titles := []string{}
	for _, group := range keys.helpColumns() {
		titles = append(titles, group.Title)
		for _, binding := range group.Bindings {
			fromColumns = append(fromColumns, binding.Help().Key+" "+binding.Help().Desc)
		}
	}
	slices.Sort(fromGroups)
	slices.Sort(fromColumns)
	if !slices.Equal(fromGroups, fromColumns) {
		t.Errorf("help columns %v differ from the key groups %v", fromColumns, fromGroups)
	}
	if !slices.Equal(titles, []string{"Navigation", "Account", "Session"}) {
		t.Errorf("column titles %v", titles)
	}
}

func TestHelpHasThreeHeadedColumns(t *testing.T) {
	out := helpScreenAt(t, 160, 40)
	lines := strings.Split(out, "\n")
	if !strings.HasPrefix(lines[0], "Keys") || strings.Contains(out, "search keys") {
		t.Errorf("title row should read Keys with no search hint:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimRight(lines[0], " "), "esc or ? to close") {
		t.Errorf("close hint must be on the right of the title row: %q", lines[0])
	}
	heading := lineWith(t, out, "navigation", "account", "session")
	if heading == "" {
		t.Fatal("the three headings must share a row")
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "providers") {
			return
		}
	}
	t.Errorf("no providers section:\n%s", out)
}

func TestHelpStacksColumnsBelow100(t *testing.T) {
	out := helpScreenAt(t, 90, 80)
	nav, account, session := lineIndex(t, out, "navigation"), lineIndex(t, out, "account"), lineIndex(t, out, "session")
	if !(nav < account && account < session) {
		t.Errorf("headings must be stacked in order, got rows %d %d %d:\n%s", nav, account, session, out)
	}
	if strings.Contains(strings.Split(out, "\n")[nav], "account") {
		t.Errorf("columns were not stacked:\n%s", out)
	}
}

func TestHelpFitsEverySize(t *testing.T) {
	for _, width := range []int{160, 120, 90, 70, 60} {
		for _, height := range []int{12, 24, 40} {
			out := helpScreenAt(t, width, height)
			lines := strings.Split(out, "\n")
			if len(lines) > height {
				t.Errorf("%dx%d: %d rows", width, height, len(lines))
			}
			for _, line := range lines {
				if w := len([]rune(line)); w > width {
					t.Errorf("%dx%d: row of %d cells: %q", width, height, w, line)
				}
			}
		}
	}
}

func TestHelpKeepsEveryBindingStacked(t *testing.T) {
	out := helpScreenAt(t, 70, 100)
	for _, group := range keys.groups() {
		for _, binding := range group.Bindings {
			h := binding.Help()
			if !strings.Contains(out, h.Desc) {
				t.Errorf("stacked help lacks %q", h.Desc)
			}
		}
	}
}

func loginColourPump(t *testing.T, width int) *loginPump {
	t.Helper()
	svc := loginFixture(t)
	svc.loginFn = blockedLogin(nil,
		step(provider.LoginStepPending, stepIsolateLabel), step(provider.LoginStepPending, stepLoginLabel),
		step(provider.LoginStepStarted, stepIsolateLabel), step(provider.LoginStepDone, stepIsolateLabel),
		step(provider.LoginStepStarted, stepLoginLabel), deviceCodeEvent, output("$ az login --use-device-code"))
	p := newLoginPump(t, svc)
	p.m = resize(t, p.m, width, 30)
	selectAccount(p, "corp-azure")
	p.key("l")
	return p
}

func TestLoginBoxSpansTheLeftPane(t *testing.T) {
	p := loginColourPump(t, 120)
	out := p.screen()
	top := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "┌") && strings.Contains(line, "┐") {
			top = line
		}
	}
	box := top[strings.Index(top, "┌") : strings.Index(top, "┐")+len("┐")]
	if got := len([]rune(box)); got < 60 {
		t.Errorf("box is %d cells wide, want the whole left pane:\n%s", got, out)
	}
	if strings.Contains(out, "1 ✓") || !strings.Contains(out, "✓ isolate config") {
		t.Errorf("steps are not numbered in the mockup:\n%s", out)
	}
}

func TestLoginKeyChips(t *testing.T) {
	p := loginColourPump(t, 120)
	p.m.profile = colorprofile.TrueColor
	st := newStylesFor(p.m.profile)
	raw := p.m.View().Content
	for _, k := range []string{"o", "y"} {
		if !strings.Contains(raw, st.Selected.Bold(true).Render(" "+k+" ")) {
			t.Errorf("key %s is not a chip", k)
		}
	}
	if !strings.Contains(raw, st.Selected.Bold(true).Render(" esc ")) {
		t.Error("esc is not a bold chip")
	}
	if !strings.Contains(raw, st.Warn.Render("⠋")) {
		t.Error("the running step needs the spinner")
	}
}

func TestLoginStackedHasNoDuplicateChrome(t *testing.T) {
	for _, width := range []int{90, 70, 60} {
		out := resizeLoginScreen(t, width)
		if n := strings.Count(out, "cancel (stops az"); n != 1 {
			t.Errorf("%d cols: cancel hint appears %d times:\n%s", width, n, out)
		}
		if n := strings.Count(out, "need action"); n > 1 {
			t.Errorf("%d cols: status line appears %d times:\n%s", width, n, out)
		}
	}
}

func resizeLoginScreen(t *testing.T, width int) string {
	t.Helper()
	return loginColourPump(t, width).screen()
}
