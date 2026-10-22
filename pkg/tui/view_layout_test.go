package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

// TestBreakpointsWidthBoundaries exercises the tier selection at each boundary
// (≥150 wide, 100-149 standard, 60-99 narrow) and the columns each tier shows.
func TestBreakpointsWidthBoundaries(t *testing.T) {
	cases := []struct {
		width        int
		height       int
		tier         tier
		wantADC      bool
		wantIdentity bool
		tooSmall     bool
	}{
		{59, 12, tierNarrow, false, false, true},   // too small
		{60, 12, tierNarrow, true, false, false},   // narrow minimum: IDENTITY drops before ADC
		{99, 12, tierNarrow, true, true, false},    // narrow maximum
		{100, 12, tierStandard, true, true, false}, // standard minimum
		{149, 12, tierStandard, true, true, false}, // standard maximum
		{150, 12, tierWide, true, true, false},     // wide minimum
		{60, 11, tierNarrow, false, false, true},   // height too small
	}
	svc := newFakeService(sampleOverview("work", "personal", "work"))
	for _, c := range cases {
		m := newTestModel(t, svc, c.width, c.height)
		if c.tooSmall {
			if !m.tooSmall() {
				t.Errorf("%dx%d: expected too small", c.width, c.height)
			}
			continue
		}
		if got := tierFor(c.width); got != c.tier {
			t.Errorf("%d: tier %v want %v", c.width, got, c.tier)
		}
		head := columnHeaderLine(t, screen(m))
		if got := strings.Contains(head, "ADC"); got != c.wantADC {
			t.Errorf("%dx%d: ADC shown=%v want %v in %q", c.width, c.height, got, c.wantADC, head)
		}
		if got := strings.Contains(head, "IDENTITY"); got != c.wantIdentity {
			t.Errorf("%dx%d: IDENTITY shown=%v want %v in %q", c.width, c.height, got, c.wantIdentity, head)
		}
	}
}

// columnHeaderLine returns the rendered line holding the NAME column header.
func columnHeaderLine(t testing.TB, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "NAME") && strings.Contains(line, "STATUS") {
			return line
		}
	}
	t.Fatalf("no column header line in:\n%s", out)
	return ""
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
	// work row shows active marker; the others do not
	g := newGlyphs(asciiRequested())
	for _, line := range strings.Split(screen(m), "\n") {
		marker := string([]rune(line + "  ")[:markerWidth])
		switch {
		case strings.Contains(line, "work@example.com"):
			if !strings.Contains(marker, g.Active) {
				t.Errorf("active marker %q not in row: %q", g.Active, line)
			}
		case strings.Contains(line, "personal@example.com"), strings.Contains(line, "sandbox@example.com"):
			if strings.Contains(marker, g.Active) {
				t.Errorf("inactive row has active marker: %q", line)
			}
		}
	}
}

// TestNoActiveAccount verifies the header shows the slot with no active account.
func TestNoActiveAccount(t *testing.T) {
	svc := newFakeService(sampleOverview("", "alpha", "beta"))
	m := newTestModel(t, svc, 100, 24)
	out := screen(m)
	if header := strings.Split(out, "\n")[0]; !strings.Contains(header, "GCP ○") || strings.Contains(header, "GCP ●") {
		t.Errorf("want 'GCP ○' (no active account) in header, got: %q", header)
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

// TestColonOpensPalette verifies ':' opens the command palette.
func TestColonOpensPalette(t *testing.T) {
	svc := newFakeService(sampleOverview("a", "a"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.dispatch(keyMsg(":"))
	if _, ok := m.modal.(*palette); !ok {
		t.Errorf("modal %T, want the palette", m.modal)
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

// TestASCIIGlyphs verifies GCTX_ASCII=1 replaces every unicode glyph.
func TestASCIIGlyphs(t *testing.T) {
	t.Setenv("GCTX_ASCII", "1")
	g := newGlyphs(asciiRequested())
	if g.Active != "*" || g.Yes != "+" || g.No != "x" || g.Warn != "!" || g.Ellipsis != "..." {
		t.Errorf("unexpected ASCII glyphs: %+v", g)
	}
	if g.Unverified != "?" || g.Missing != "-" || g.Linked != "->" {
		t.Errorf("unexpected ASCII health glyphs: %+v", g)
	}
	all := append([]string{g.Active, g.Yes, g.No, g.Warn, g.Ellipsis, g.HRule, g.Unverified, g.Missing, g.Linked,
		g.Bar, g.VSep, g.Dot, g.Enter}, g.Spinner...)
	if len(g.Spinner) < 2 {
		t.Errorf("spinner frames %v", g.Spinner)
	}
	for _, glyph := range all {
		for _, r := range glyph {
			if r > 127 {
				t.Errorf("non-ASCII glyph %q in ASCII set", glyph)
			}
		}
	}
	t.Setenv("GCTX_ASCII", "")
	if g := newGlyphs(asciiRequested()); g.Unverified != "?" || g.Missing != "○" || g.Linked != "↳" {
		t.Errorf("unicode glyphs: %+v", g)
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
	m := newTestModel(t, newFakeService(sampleOverview("work", "work")), 70, 24)
	hints := renderHints(m, 70, newStyles())
	if n := len(strings.Split(strings.TrimSpace(ansi.Strip(hints)), "  ")); n > 5 || strings.Contains(hints, "login") {
		t.Errorf("narrow hints should be at most 5, without login: %q", hints)
	}
}

// TestHintLineWide verifies full hints at wide width.
func TestHintLineWide(t *testing.T) {
	m := newTestModel(t, newFakeService(sampleOverview("work", "work")), 100, 24)
	hints := renderHints(m, 100, newStyles())
	if !strings.Contains(hints, "filter") {
		t.Errorf("wide hints should mention filter: %q", hints)
	}
}

func TestHintLineKeepsHelpAndQuit(t *testing.T) {
	for _, width := range []int{160, 120, 100, 90, 70, 60} {
		m := newTestModel(t, newMultiService(), width, 30)
		line := ansi.Strip(renderHints(m, width, newStyles()))
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("width %d: line is %d cells: %q", width, w, line)
		}
		if !strings.HasSuffix(line, "? help  q quit") {
			t.Errorf("width %d: line does not end with help and quit: %q", width, line)
		}
	}
}
