package tui

import (
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/manager"
)

// TestTooSmallMessage verifies the too-small replacement screen.
func TestTooSmallMessage(t *testing.T) {
	m := newTestModel(t, newFakeService(sampleOverview("work", "work")), 40, 10)
	got := screen(m)
	if !strings.Contains(got, "Terminal too small") {
		t.Errorf("want too-small message, got: %q", got)
	}
}

// TestBreakpointsWidthBoundaries exercises the column-set selection at each boundary.
func TestBreakpointsWidthBoundaries(t *testing.T) {
	cases := []struct {
		width    int
		height   int
		wantADC  bool
		wantEmail bool
		tooSmall bool
	}{
		{59, 12, false, false, true},  // too small
		{60, 12, false, false, false}, // tier2 minimum
		{89, 12, false, false, false}, // tier2 maximum
		{90, 12, false, true, false},  // tier1 minimum
		{119, 12, false, true, false}, // tier1 maximum
		{120, 12, true, true, false},  // tier0 minimum
		{60, 11, false, false, true},  // height too small
	}
	svc := newFakeService(sampleOverview("work", "personal", "work"))
	for _, c := range cases {
		m := newTestModel(t, svc, c.width, c.height)
		col := layoutColumns(c.width)
		if c.tooSmall {
			if !m.tooSmall() {
				t.Errorf("%dx%d: expected too small", c.width, c.height)
			}
			continue
		}
		if col.showADC != c.wantADC {
			t.Errorf("%dx%d: showADC=%v want %v", c.width, c.height, col.showADC, c.wantADC)
		}
		if col.showEmail != c.wantEmail {
			t.Errorf("%dx%d: showEmail=%v want %v", c.width, c.height, col.showEmail, c.wantEmail)
		}
	}
}

// TestActiveHighlightAndOrder checks the active marker and sorted order.
func TestActiveHighlightAndOrder(t *testing.T) {
	// accounts: personal, sandbox, work; work is active
	svc := newFakeService(sampleOverview("work", "personal", "sandbox", "work"))
	m := newTestModel(t, svc, 100, 24)
	// accounts list is sorted by name; active is work
	names := make([]string, len(m.visible))
	for i, idx := range m.visible {
		names[i] = m.overview.Accounts[idx].Name
	}
	if len(names) != 3 {
		t.Fatalf("visible count %d, want 3", len(names))
	}
	if names[0] != "personal" || names[1] != "sandbox" || names[2] != "work" {
		t.Errorf("wrong order: %v", names)
	}
	// initial selection is active account
	if m.selected != "work" {
		t.Errorf("selected %q, want work", m.selected)
	}
	// work row shows active marker
	g := newGlyphs(asciiRequested())
	out := renderRow(m.overview.Accounts[m.visible[2]], layoutColumns(100), g, newStyles(), false)
	if !strings.Contains(out, g.Active) {
		t.Errorf("active marker %q not in row: %q", g.Active, out)
	}
}

// TestNoActiveAccount verifies the header shows "Active: none".
func TestNoActiveAccount(t *testing.T) {
	svc := newFakeService(sampleOverview("", "alpha", "beta"))
	m := newTestModel(t, svc, 100, 24)
	out := screen(m)
	if !strings.Contains(out, "no active account") {
		t.Errorf("want 'no active account', got: %q", out)
	}
	// selection should be on first row
	if m.selected != "alpha" {
		t.Errorf("selected %q, want alpha", m.selected)
	}
}

// TestEmptyAccountList verifies the empty state message.
func TestEmptyAccountList(t *testing.T) {
	svc := newFakeService(manager.Overview{GcloudAvailable: true})
	m := newTestModel(t, svc, 100, 24)
	out := screen(m)
	if !strings.Contains(out, "No accounts") {
		t.Errorf("want empty state message, got: %q", out)
	}
}

// TestErrorState verifies the error state when config is unreadable.
func TestErrorState(t *testing.T) {
	svc := &fakeService{
		overviewErr: errTest("config unreadable"),
		gcloudBin:   "gcloud",
		gates:       map[string]chan struct{}{},
	}
	m := newTestModel(t, svc, 100, 24)
	out := screen(m)
	if !strings.Contains(out, "Error loading") {
		t.Errorf("want error state, got: %q", out)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// TestNavigationNoBoundsWrap verifies no wrap at first/last row.
func TestNavigationNoBoundsWrap(t *testing.T) {
	svc := newFakeService(sampleOverview("a", "a", "b", "c"))
	m := newTestModel(t, svc, 100, 24)
	// move to first row and try to go up
	m = press(t, m, "g") // go to first
	before := m.selected
	m = press(t, m, "k")
	if m.selected != before {
		t.Errorf("wrapped up: was %q, now %q", before, m.selected)
	}
	// move to last row and try to go down
	m = press(t, m, "G")
	before = m.selected
	m = press(t, m, "j")
	if m.selected != before {
		t.Errorf("wrapped down: was %q, now %q", before, m.selected)
	}
}

// TestNavigationGCapital verifies G moves to last row (not the letter 'G' interpreted as nav).
func TestNavigationGCapital(t *testing.T) {
	// 40 accounts to test scrolling
	names := make([]string, 40)
	for i := range names {
		names[i] = "account" + string(rune('a'+i%26))
	}
	svc := newFakeService(sampleOverview("accounta", names...))
	m := newTestModel(t, svc, 100, 24)
	m = press(t, m, "G")
	last := m.overview.Accounts[m.visible[len(m.visible)-1]].Name
	if m.selected != last {
		t.Errorf("after G: selected %q, want %q", m.selected, last)
	}
}

// TestFilterNarrowsAndKeepsSelection tests filtering behavior.
func TestFilterNarrowsAndKeepsSelection(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "personal", "sandbox", "work"))
	m := newTestModel(t, svc, 100, 24)
	// open filter and type "work"
	m = press(t, m, "/", "w", "o", "r", "k", "enter")
	if len(m.visible) != 1 {
		t.Errorf("visible %d, want 1", len(m.visible))
	}
	if m.selected != "work" {
		t.Errorf("selected %q, want work", m.selected)
	}
	// title shows filter info
	out := screen(m)
	if !strings.Contains(out, "filter:") {
		t.Errorf("filter title missing: %q", out)
	}
}

// TestFilterJDoesNotMove verifies 'j' in filter adds to query, not navigation.
func TestFilterJDoesNotMove(t *testing.T) {
	svc := newFakeService(sampleOverview("a", "a", "b", "c"))
	m := newTestModel(t, svc, 100, 24)
	before := m.selected
	m = press(t, m, "/", "j")
	if m.selected != before {
		t.Errorf("j in filter moved selection: %q -> %q", before, m.selected)
	}
	if m.filter.input.Value() != "j" {
		t.Errorf("filter input %q, want 'j'", m.filter.input.Value())
	}
}

// TestFilterEscClearsAndRestores tests that esc in filter clears the query.
func TestFilterEscClearsAndRestores(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "personal", "sandbox", "work"))
	m := newTestModel(t, svc, 100, 24)
	m = press(t, m, "/", "s", "a", "n", "d", "enter") // filter to sandbox
	totalBefore := len(m.overview.Accounts)
	if len(m.visible) == totalBefore {
		t.Error("filter did not narrow")
	}
	m = press(t, m, "esc") // clear filter
	if len(m.visible) != totalBefore {
		t.Errorf("filter not cleared: visible %d, total %d", len(m.visible), totalBefore)
	}
}

// TestFilterNoMatchEmptyState verifies the no-match message.
func TestFilterNoMatchEmptyState(t *testing.T) {
	svc := newFakeService(sampleOverview("a", "alpha", "beta"))
	m := newTestModel(t, svc, 100, 24)
	m = press(t, m, "/", "z", "z", "z", "enter")
	out := screen(m)
	if !strings.Contains(out, "No accounts match") {
		t.Errorf("expected no-match message, got: %q", out)
	}
}

// TestReservedColon verifies the ':' reserved message.
func TestReservedColon(t *testing.T) {
	svc := newFakeService(sampleOverview("a", "a"))
	m := newTestModel(t, svc, 100, 24)
	// Use dispatch directly to avoid running the status-expiry timer in drain.
	m, _ = m.dispatch(keyMsg(":"))
	if m.status.text == "" {
		t.Error("expected status message after :")
	}
	if !strings.Contains(m.status.text, "Command mode") {
		t.Errorf("wrong status: %q", m.status.text)
	}
}

// TestHelpScreenOpen verifies ? opens the help overlay.
func TestHelpScreenOpen(t *testing.T) {
	svc := newFakeService(sampleOverview("a", "a"))
	m := newTestModel(t, svc, 100, 24)
	m = press(t, m, "?")
	if m.screen != screenHelp {
		t.Errorf("screen %v, want screenHelp", m.screen)
	}
	out := screen(m)
	// help should list every binding
	for _, group := range keys.groups() {
		for _, binding := range group.Bindings {
			desc := binding.Help().Desc
			if desc != "" && !strings.Contains(out, desc) {
				t.Errorf("help missing description for binding %q", desc)
			}
		}
	}
}

// TestHelpScreenClose verifies esc/q closes help.
func TestHelpScreenClose(t *testing.T) {
	svc := newFakeService(sampleOverview("a", "a"))
	m := newTestModel(t, svc, 100, 24)
	m = press(t, m, "?")
	m = press(t, m, "esc")
	if m.screen != screenMain {
		t.Errorf("screen %v, want screenMain after esc", m.screen)
	}
}

// TestDetailsViewOpen verifies d/i opens details overlay.
func TestDetailsViewOpen(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "sandbox", "work"))
	m := newTestModel(t, svc, 100, 24)
	m.selected = "sandbox"
	m = press(t, m, "d")
	if m.screen != screenDetails {
		t.Errorf("screen %v, want screenDetails", m.screen)
	}
	out := screen(m)
	if !strings.Contains(out, "sandbox") {
		t.Errorf("details missing account name: %q", out)
	}
}

// TestDetailsViewFindings verifies findings and remedies are shown.
func TestDetailsViewFindings(t *testing.T) {
	overview := sampleOverview("work", "missing-adc", "work")
	// make missing-adc have no ADC
	for i, a := range overview.Accounts {
		if a.Name == "missing-adc" {
			overview.Accounts[i].ADCExists = false
			overview.Accounts[i].Status = manager.HealthADCMissing
			overview.Accounts[i].Findings = []manager.Finding{{Health: manager.HealthADCMissing}}
		}
	}
	svc := newFakeService(overview)
	m := newTestModel(t, svc, 100, 24)
	m.selected = "missing-adc"
	m = press(t, m, "d")
	out := screen(m)
	if !strings.Contains(out, "ADC MISSING") {
		t.Errorf("missing ADC MISSING in details: %q", out)
	}
	if !strings.Contains(out, "press l to log in") {
		t.Errorf("missing remedy in details: %q", out)
	}
}

// TestASCIIMode verifies GCTX_ASCII replaces unicode glyphs.
func TestASCIIGlyphs(t *testing.T) {
	g := newGlyphs(true)
	if g.Active != "*" || g.Yes != "+" || g.No != "x" || g.Warn != "!" || g.Ellipsis != "..." {
		t.Errorf("unexpected ASCII glyphs: %+v", g)
	}
}

// TestRandomResizeNoPanic verifies resizing does not panic.
func TestRandomResizeNoPanic(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "personal", "sandbox", "work"))
	m := newTestModel(t, svc, 100, 24)
	sizes := [][2]int{{40, 10}, {120, 40}, {59, 11}, {60, 12}, {80, 24}, {200, 60}}
	for _, sz := range sizes {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic at %dx%d: %v", sz[0], sz[1], r)
				}
			}()
			m = resize(t, m, sz[0], sz[1])
			_ = screen(m)
		}()
	}
}

// TestHintLineNarrow verifies shortened hints at narrow width.
func TestHintLineNarrow(t *testing.T) {
	hints := renderHints(Model{}, layoutColumns(70), newStyles())
	if strings.Contains(hints, "filter") {
		t.Errorf("narrow hints should not mention filter: %q", hints)
	}
}

// TestHintLineWide verifies full hints at wide width.
func TestHintLineWide(t *testing.T) {
	hints := renderHints(Model{}, layoutColumns(100), newStyles())
	if !strings.Contains(hints, "filter") {
		t.Errorf("wide hints should mention filter: %q", hints)
	}
}
