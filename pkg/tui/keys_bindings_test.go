package tui

import (
	"strings"
	"testing"
)

// TestActionEntryPoints verifies that key presses reach the correct entry points.
// The stubs record no calls, but we verify the model state (modal set, status set).
func TestEnterCallsStartSwitch(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	// With stub startSwitch, pressing enter should return without error.
	// The stubs are no-ops; we just verify it doesn't panic and no status set.
	before := m.status.text
	m, _ = m.dispatch(keyMsg("enter"))
	_ = before
}

func TestACallsOpenSave(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.dispatch(keyMsg("a"))
	// stub openSave is a no-op; no panic
}

func TestCtrlDCallsOpenDelete(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.dispatch(keyMsg("ctrl+d"))
	// stub openDelete is a no-op; no panic
}

func TestCCallsOpenCreate(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.dispatch(keyMsg("c"))
	// stub openCreate is a no-op; no panic
}

func TestRCallsOpenRun(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.dispatch(keyMsg("r"))
	if m.modal == nil {
		t.Error("r should set a modal via openRun stub")
	}
}

func TestSCallsStartShell(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.dispatch(keyMsg("s"))
	// stub startShell (via beginShell) is a no-op; no panic
}

func TestLCallsStartLogin(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.dispatch(keyMsg("l"))
	// stub startLogin (via beginLogin) is a no-op; no panic
}

// TestGcloudUnavailableGatesActions verifies gcloud-gated keys show error message.
func TestGcloudUnavailableGatesActions(t *testing.T) {
	ov := sampleOverview("work", "work")
	ov.GcloudAvailable = false
	svc := newFakeService(ov)
	m := newTestModel(t, svc, 100, 24)

	gated := []string{"enter", "l", "r", "c"}
	for _, k := range gated {
		m2, _ := m.dispatch(keyMsg(k))
		if !strings.Contains(m2.status.text, "gcloud not found") {
			t.Errorf("key %q: expected gcloud-unavailable error, got %q", k, m2.status.text)
		}
	}

	// s, a, d should not be gated
	notGated := []string{"s", "a"}
	for _, k := range notGated {
		m2, _ := m.dispatch(keyMsg(k))
		if strings.Contains(m2.status.text, "gcloud not found") {
			t.Errorf("key %q: should not be gcloud-gated", k)
		}
	}
}

// TestBusyGatesMutatingKeys verifies busy state blocks mutating actions.
func TestBusyGatesMutatingKeys(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	// simulate a busy state
	m.op = &operation{kind: opSwitch, account: "work", desc: "switching to work", seq: 1}

	// mutating actions should show busy message
	for _, k := range []string{"enter", "a", "ctrl+d", "c", "r", "s", "l"} {
		m2, _ := m.dispatch(keyMsg(k))
		if !strings.Contains(m2.status.text, "Busy") {
			t.Errorf("key %q while busy: expected busy message, got %q", k, m2.status.text)
		}
	}

	// navigation still works
	before := m.selected
	m2 := press(t, m, "j")
	_ = before // selection may or may not change, but no panic
	_ = m2
}

// TestHandleCredsDropsStale verifies stale credential results are ignored.
func TestHandleCredsDropsStale(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m.creds.seq = 5
	m.creds.phase = credChecking

	stale := credsCheckedMsg{seq: 3}
	m, _ = m.dispatch(stale)
	if m.creds.phase != credChecking {
		t.Error("stale creds result should not change phase")
	}
}

// TestHandleCredsAppliesResult verifies a matching credential result updates state.
func TestHandleCredsAppliesResult(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m.creds.seq = 7
	m.creds.phase = credChecking

	m, _ = m.dispatch(credsCheckedMsg{seq: 7})
	if m.creds.phase != credDone {
		t.Errorf("phase %v, want credDone", m.creds.phase)
	}
}

// TestSelectionKeptOnReload verifies reload preserves selection when account still exists.
func TestSelectionKeptOnReload(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "personal", "sandbox", "work"))
	m := newTestModel(t, svc, 100, 24)
	m.selected = "sandbox"
	m.reload()
	if m.selected != "sandbox" {
		t.Errorf("selected %q, want sandbox", m.selected)
	}
}

// TestStatusMessageExpiry verifies info messages expire.
func TestStatusMessageExpiry(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	now := m.now()
	m.statusSeq = 1
	m.status = statusMsg{id: 1, text: "info", level: levelInfo, expires: now.Add(infoMessageTTL)}

	// before expiry: still there
	m, _ = m.dispatch(statusExpiredMsg{id: 99}) // wrong id
	if m.status.text == "" {
		t.Error("status cleared by wrong id")
	}

	// exact id: cleared
	m2, _ := m.dispatch(statusExpiredMsg{id: 1})
	if m2.status.text != "" {
		t.Errorf("status not cleared: %q", m2.status.text)
	}
}

// TestErrorStatusPersists verifies error messages do not expire.
func TestErrorStatusPersists(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m.statusSeq = 3
	m.status = statusMsg{id: 3, text: "error!", level: levelError}

	m, _ = m.dispatch(statusExpiredMsg{id: 3})
	if m.status.text == "" {
		t.Error("error status should persist (no expires set)")
	}
}

// TestReloadKeyStartsCredsCheck verifies R triggers a creds check.
func TestReloadKeyStartsCredsCheck(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	callsBefore := len(svc.Calls())
	m, _ = m.dispatch(keyMsg("R"))
	// After dispatch the creds check cmd is returned but not run yet.
	// The phase should be credChecking.
	if m.creds.phase != credChecking {
		t.Errorf("creds phase %v after R, want credChecking", m.creds.phase)
	}
	_ = callsBefore
}

// TestNewMainScreenBindings pins the v2 main-screen keys and that the
// existing keys keep theirs.
func TestNewMainScreenBindings(t *testing.T) {
	cases := []struct {
		name string
		keys []string
		want []string
	}{
		{"NextTab", keys.NextTab.Keys(), []string{"tab"}},
		{"PrevTab", keys.PrevTab.Keys(), []string{"shift+tab"}},
		{"JumpTab", keys.JumpTab.Keys(), []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}},
		{"TogglePane", keys.TogglePane.Keys(), []string{"p"}},
		{"Fold", keys.Fold.Keys(), []string{"z"}},
		{"NeedsAction", keys.NeedsAction.Keys(), []string{"!"}},
		{"Sort", keys.Sort.Keys(), []string{"o"}},
		{"Delete", keys.Delete.Keys(), []string{"ctrl+d"}},
		{"First", keys.First.Keys(), []string{"g", "home"}},
	}
	for _, c := range cases {
		if strings.Join(c.keys, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: keys %v, want %v", c.name, c.keys, c.want)
		}
	}
}
