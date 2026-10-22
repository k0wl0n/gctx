package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestQuitReturnsTeaQuit(t *testing.T) {
	m := newTestModel(t, newFakeService(sampleOverview("work", "personal", "work")), 100, 24)
	for _, key := range []string{"q", "ctrl+c"} {
		_, cmd := m.dispatch(keyMsg(key))
		if cmd == nil {
			t.Fatalf("%s: no command", key)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s did not quit", key)
		}
	}
}

func TestInitialSelectionIsActiveAccount(t *testing.T) {
	m := newTestModel(t, newFakeService(sampleOverview("work", "personal", "sandbox", "work")), 100, 24)
	if m.selected != "work" || len(m.visible) != 3 {
		t.Errorf("selected %q visible %v", m.selected, m.visible)
	}
}

func TestInitStartsCredentialCheckAfterFirstFrame(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	if got := svc.Calls(); len(got) != 1 || got[0] != "Overview" {
		t.Fatalf("calls before first frame: %v", got)
	}
	m = drain(t, m, m.Init())
	// One credential check per provider with accounts, issued only by Init.
	calls := svc.Calls()
	if len(calls) < 2 || calls[0] != "Overview" || !slices.Contains(calls, "CheckProviderCredentials gcp") {
		t.Errorf("calls %s", strings.Join(calls, ","))
	}
}

func TestViewRequestsAltScreenAndFocus(t *testing.T) {
	view := newTestModel(t, newFakeService(sampleOverview("")), 100, 24).View()
	if !view.AltScreen || !view.ReportFocus {
		t.Errorf("view %+v", view)
	}
}

func TestKeyTableIsComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, group := range keys.groups() {
		for _, binding := range group.Bindings {
			if len(binding.Keys()) == 0 || binding.Help().Desc == "" {
				t.Errorf("incomplete binding in %s: %+v", group.Title, binding)
			}
			for _, k := range binding.Keys() {
				if seen[k] {
					t.Errorf("key %q bound twice", k)
				}
				seen[k] = true
			}
		}
	}
	for _, want := range []string{"j", "G", "ctrl+d", "R", "q", "?", ":", "enter"} {
		if !seen[want] {
			t.Errorf("key %q missing", want)
		}
	}
}

func TestGlyphSets(t *testing.T) {
	if g := newGlyphs(true); g.Active != "*" || g.Ellipsis != "..." {
		t.Errorf("%+v", g)
	}
	if g := newGlyphs(false); g.Active != "●" {
		t.Errorf("%+v", g)
	}
}

func TestTokenizeStub(t *testing.T) {
	args, err := tokenize("config get-value project", "gcloud", "linux")
	if err != nil || len(args) != 3 {
		t.Errorf("%v %v", args, err)
	}
}

func TestExecStubsImplementExecCommand(t *testing.T) {
	var _ tea.ExecCommand = (*loginExec)(nil)
	var _ tea.ExecCommand = (*runExec)(nil)
	var _ tea.ExecCommand = (*shellExec)(nil)
}

func TestModalAndExecEntryPoints(t *testing.T) {
	m := newTestModel(t, newFakeService(sampleOverview("work", "work")), 100, 24)
	m, _ = m.openRun("work")
	if m.modal == nil {
		t.Error("openRun did not set a modal")
	}
	m.modal = nil
	if _, cmd := m.startLogin("work"); cmd == nil {
		t.Error("beginLogin should return a non-nil command")
	}
}

func TestLineInputEditing(t *testing.T) {
	in := newLineInput("> ")
	for _, k := range []string{"a", "b", "c", "left", "X", "backspace", "backspace"} {
		in.update(keyMsg(k))
	}
	if in.Value() != "ac" || in.Cursor() != 1 {
		t.Errorf("value %q cursor %d", in.Value(), in.Cursor())
	}
	if in.update(keyMsg("enter")) && in.Value() != "ac" {
		t.Error("enter must not edit the value")
	}
}

func TestMapRunError(t *testing.T) {
	if mapRunError(nil) != nil || mapRunError(tea.ErrInterrupted) != nil || mapRunError(tea.ErrProgramKilled) != nil {
		t.Error("clean exits must map to nil")
	}
	if err := mapRunError(tea.ErrProgramPanic); err == nil || !strings.HasPrefix(err.Error(), "interactive UI failed: ") {
		t.Errorf("%v", err)
	}
}

func TestKeyMessagesMatchRuntimeStrings(t *testing.T) {
	for name, want := range map[string]string{"space": "space", "G": "G", "ctrl+d": "ctrl+d", "enter": "enter", "esc": "esc", "pgdown": "pgdown", "?": "?"} {
		if got := keyMsg(name).String(); got != want {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestTooSmallGateOnlyAllowsQuit(t *testing.T) {
	m := newTestModel(t, newFakeService(sampleOverview("work", "a", "work")), 40, 10)
	before := m.selected
	m = press(t, m, "j", "?")
	if m.selected != before || m.screen != screenMain {
		t.Error("keys acted while too small")
	}
	if _, cmd := m.dispatch(keyMsg("q")); cmd == nil {
		t.Error("q must still quit")
	}
}
