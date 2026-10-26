package tui

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// multiModel is the canvas fixture after its first frame: envs loaded and
// every provider checked.
func multiModel(t testing.TB, svc *fakeService, width, height int) Model {
	t.Helper()
	m := newTestModel(t, svc, width, height)
	m.now = func() time.Time { return fixtureNow }
	return drain(t, m, m.Init())
}

// checksInFlight starts a credential check without running it, loads the
// envs, then finishes the given providers' checks from the fixture reports.
func checksInFlight(t testing.TB, svc *fakeService, width, height int, finished ...provider.ID) Model {
	t.Helper()
	m := newTestModel(t, svc, width, height)
	m.now = func() time.Time { return fixtureNow }
	m = drain(t, m, m.loadEnvCmd())
	m, _ = m.dispatch(startCredsMsg{})
	for _, id := range finished {
		m, _ = m.dispatch(credsCheckedMsg{seq: m.creds.seq, provider: id, report: svc.providerReports[id]})
	}
	return m
}

func (m Model) svcCalls() []string { return m.svc.(*fakeService).Calls() }

func lines(out string) []string { return strings.Split(out, "\n") }

// lineWith returns the first rendered line containing every needle.
func lineWith(t testing.TB, out string, needles ...string) string {
	t.Helper()
	for _, line := range lines(out) {
		all := true
		for _, n := range needles {
			all = all && strings.Contains(line, n)
		}
		if all {
			return line
		}
	}
	t.Fatalf("no line with %q in:\n%s", needles, out)
	return ""
}

func hasLineWith(out string, needles ...string) bool {
	for _, line := range lines(out) {
		all := true
		for _, n := range needles {
			all = all && strings.Contains(line, n)
		}
		if all {
			return true
		}
	}
	return false
}

// inOrder asserts that each needle appears after the previous one.
func inOrder(t testing.TB, s string, needles ...string) {
	t.Helper()
	at := 0
	for _, n := range needles {
		i := strings.Index(s[at:], n)
		if i < 0 {
			t.Fatalf("%q missing or out of order after offset %d in:\n%s", n, at, s)
		}
		at += i + len(n)
	}
}

func visibleNames(m Model) []string {
	var names []string
	for _, idx := range m.visible {
		names = append(names, m.overview.Accounts[idx].Name)
	}
	return names
}

func TestHeaderShowsOneSlotPerProviderWithAccounts(t *testing.T) {
	m := multiModel(t, newMultiService(), 160, 40)
	header := lines(screen(m))[0]
	inOrder(t, header, "gctx", "GCP ● cloudsuite", "AWS ● prod-aws", "AZR ○", "│ ai", "CLD ● claude-work", "OAI ● codex-work", "SMP ● sumopod-dev")
	// SumoPod and OpenRouter share one slot: one header entry.
	if strings.Contains(header, "ORT") {
		t.Errorf("shared slot shown twice: %q", header)
	}
	if !strings.Contains(header, "✓ cfg") || !strings.Contains(header, "auth checked") {
		t.Errorf("config health or check age missing: %q", header)
	}
}

func TestHeaderHidesEmptySlotsButKeepsTheTab(t *testing.T) {
	overview := multiOverview()
	overview.Providers = append(overview.Providers, manager.ProviderInfo{
		ID: "tencent", Label: "Tencent", Category: provider.CategoryCloud, Slot: "tencent", TargetLabel: "PROFILE",
		Badge: provider.Badge{Text: "TCT", Style: provider.BadgeFilled, Color: "#c39bff"},
		CLI:   provider.CLIStatus{Name: "tccli", Found: true}, // a missing CLI gets a tab; see TestMissingCLITabShowsInstallHint
	})
	svc := newMultiService()
	svc.overview = overview
	m := multiModel(t, svc, 160, 40)
	out := screen(m)
	if strings.Contains(lines(out)[0], "TCT") || strings.Contains(lines(out)[0], "Tencent") {
		t.Errorf("provider without accounts has a header slot:\n%s", out)
	}
	if !strings.Contains(strings.Join(lines(out)[1:3], "\n"), "Tencent 0") {
		t.Errorf("provider without accounts needs a tab with count 0:\n%s", out)
	}
}

func TestHeaderEnvLine(t *testing.T) {
	svc := newMultiService()
	svc.envs["sumopod-dev"] = append(svc.envs["sumopod-dev"], "SUMOPOD_TOKEN=sk-live-0123456789")
	m := multiModel(t, svc, 160, 40)
	m.width = 400 // room for the whole env line
	out := screen(m)
	env := lines(out)[1]
	if !strings.HasPrefix(env, "env ") {
		t.Fatalf("second line %q", env)
	}
	inOrder(t, env, "CLOUDSDK_ACTIVE_CONFIG_NAME=cloudsuite", "AWS_PROFILE=prod-sso", "CLAUDE_CONFIG_DIR=", "CODEX_HOME=",
		"OPENAI_BASE_URL=https://ai.sumopod.com/v1", "OPENAI_API_KEY=‹op://Private/sumopod-dev/credential›")
	if strings.Contains(env, "AWS_PROFILE=legacy") {
		t.Errorf("inactive account's env in header: %q", env)
	}
	if strings.Contains(m.View().Content, "sk-live-0123456789") {
		t.Error("key-like env value rendered")
	}
	if !strings.Contains(env, "SUMOPOD_TOKEN=‹hidden›") {
		t.Errorf("secret-named var not masked: %q", env)
	}
}

func tabRow(t testing.TB, m Model) string {
	t.Helper()
	return lineWith(t, screen(m), "All ")
}

func TestTabsCycleAndJump(t *testing.T) {
	m := multiModel(t, newMultiService(), 160, 40)
	inOrder(t, tabRow(t, m), "All 19", "GCP 9", "AWS 2", "Azure 1", "│", "Claude Code 3", "Codex 1", "│", "SumoPod 2", "OpenRouter 1")

	m = press(t, m, "tab")
	if m.tab != 1 || len(m.visible) != 9 {
		t.Errorf("tab %d visible %v", m.tab, visibleNames(m))
	}
	m = press(t, m, "shift+tab", "shift+tab")
	if m.tab != 7 || !slices.Equal(visibleNames(m), []string{"openrouter"}) {
		t.Errorf("shift+tab wrap: tab %d visible %v", m.tab, visibleNames(m))
	}
	m = press(t, m, "3")
	if m.tab != 2 || !slices.Equal(visibleNames(m), []string{"legacy-aws", "prod-aws"}) {
		t.Errorf("jump 3: tab %d visible %v", m.tab, visibleNames(m))
	}
	m = press(t, m, "9") // no ninth tab
	if m.tab != 2 {
		t.Errorf("jump past last tab moved to %d", m.tab)
	}
	if !slices.Contains(visibleNames(m), m.selected) {
		t.Errorf("selection %q not on the tab", m.selected)
	}

	// The tab filters first, then the query.
	m = press(t, m, "/", "l", "e", "g", "enter")
	if !slices.Equal(visibleNames(m), []string{"legacy-aws"}) {
		t.Errorf("tab+query: %v", visibleNames(m))
	}
	m = press(t, m, "esc", "/", "c", "l", "o", "u", "d", "enter")
	if len(m.visible) != 0 {
		t.Errorf("query escaped the tab: %v", visibleNames(m))
	}
	m = press(t, m, "esc", "1")
	if m.tab != 0 || len(m.visible) != 19 {
		t.Errorf("back to All: tab %d visible %d", m.tab, len(m.visible))
	}
}

func TestTabDimmedWhenCLIMissing(t *testing.T) {
	svc := newMultiService()
	for i, info := range svc.overview.Providers {
		if info.Label == "Azure" {
			svc.overview.Providers[i].CLI.Found = false
		}
	}
	m := multiModel(t, svc, 160, 40)
	row := tabRow(t, m)
	if !strings.Contains(row, "Azure n/a") || strings.Contains(row, "Azure 1") {
		t.Errorf("tab row %q", row)
	}
	// The label is drawn dim.
	raw := lineWith(t, m.View().Content, "Azure")
	if want := newStylesFor(m.profile).Dim.Render("Azure n/a"); !strings.Contains(raw, want) {
		t.Errorf("Azure tab not dim: %q", raw)
	}
}

func TestAllTabGroupsByCategoryThenProvider(t *testing.T) {
	m := multiModel(t, newMultiService(), 160, 40)
	out := screen(m)
	inOrder(t, out, "CLOUDS", "GCP · project", "cloudsuite", "AWS · profile", "prod-aws", "AZURE · subscription", "corp-azure",
		"AI TOOLS", "CLAUDE CODE · org", "claude-work", "CODEX · workspace", "codex-work",
		"AI GATEWAYS", "SUMOPOD · base url", "sumopod-dev", "OPENROUTER · base url", "openrouter")
	names := visibleNames(m)
	if names[0] != "byoip-lab" || names[9] != "legacy-aws" || names[12] != "claude-personal" || names[18] != "openrouter" {
		t.Errorf("row order %v", names)
	}
}

func TestGroupSummaryCounts(t *testing.T) {
	m := multiModel(t, newMultiService(), 160, 40)
	out := screen(m)
	if line := lineWith(t, out, "GCP · project"); !strings.Contains(line, "9 · 7 ready, 1 expired, 1 login") {
		t.Errorf("gcp heading %q", line)
	}
	if line := lineWith(t, out, "SUMOPOD · base url"); !strings.Contains(line, "2 · 1 ready, 1 low budget") {
		t.Errorf("sumopod heading %q", line)
	}
	if line := lineWith(t, out, "CLOUDS ─"); !strings.Contains(line, "─ 12") {
		t.Errorf("category heading %q", line)
	}
}

func TestFoldShowsOnlyRowsNeedingAction(t *testing.T) {
	m := multiModel(t, newMultiService(), 160, 40)
	if m.selected != "cloudsuite" {
		t.Fatalf("selected %q", m.selected)
	}
	m = press(t, m, "z")
	if !m.folded[string(provider.GCP)] {
		t.Fatal("gcp not folded")
	}
	var gcpRows []string
	for _, idx := range m.visible {
		if a := m.overview.Accounts[idx]; a.Provider == provider.GCP {
			gcpRows = append(gcpRows, a.Name)
		}
	}
	if !slices.Equal(gcpRows, []string{"byoip-lab", "sbx-aliz"}) {
		t.Errorf("folded rows %v", gcpRows)
	}
	if m.selected != foldKey(provider.GCP) {
		t.Errorf("selection %q, want the group's placeholder", m.selected)
	}
	out := screen(m)
	if !hasLineWith(out, "… 7 more ready") || hasLineWith(out, "infra-hub-prd") {
		t.Errorf("folded group:\n%s", out)
	}
	m = press(t, m, "z")
	if m.folded[string(provider.GCP)] || len(m.visible) != 19 || m.selected != "byoip-lab" {
		t.Errorf("unfold: selected %q visible %v", m.selected, visibleNames(m))
	}
}

func TestSingleProviderTabDropsKindAddsProviderColumns(t *testing.T) {
	m := multiModel(t, newMultiService(), 120, 30)
	if head := columnHeaderLine(t, screen(m)); !strings.Contains(head, "KIND") {
		t.Errorf("All tab header %q", head)
	}
	m = press(t, m, "3")
	out := screen(m)
	head := columnHeaderLine(t, out)
	if strings.Contains(head, "KIND") || !strings.Contains(head, "PROFILE") || !strings.Contains(head, "REGION") {
		t.Errorf("AWS tab header %q", head)
	}
	if row := lineWith(t, out, "prod-aws", "prod-sso"); !strings.Contains(row, "eu-west-1") {
		t.Errorf("region missing: %q", row)
	}
	if hasLineWith(out, "AWS · profile") {
		t.Error("single-provider tab shows group headings")
	}
}

func TestDetailPaneAtWideOnly(t *testing.T) {
	svc := newMultiService()
	m := multiModel(t, svc, 160, 40)
	m.selected = "prod-aws"
	if !m.paneOpen || !strings.Contains(screen(m), "env applied on activate") {
		t.Fatalf("no pane at 160:\n%s", screen(m))
	}
	pane := screen(m)
	for _, want := range []string{"region", "eu-west-1", "AWS_PROFILE=prod-sso", `eval "$(gctx env prod-aws)"`} {
		if !strings.Contains(pane, want) {
			t.Errorf("pane missing %q", want)
		}
	}
	m = press(t, m, "p")
	if m.paneOpen || strings.Contains(screen(m), "env applied on activate") {
		t.Error("p did not close the pane")
	}

	m = resize(t, multiModel(t, svc, 160, 40), 120, 40)
	m.selected = "prod-aws"
	if strings.Contains(screen(m), "env applied on activate") {
		t.Error("pane at 120")
	}
	m = press(t, m, "d")
	if m.screen != screenDetails || !strings.Contains(screen(m), "eu-west-1") {
		t.Errorf("d at 120: screen %v\n%s", m.screen, screen(m))
	}
}

func statusLine(m Model) string {
	all := lines(screen(m))
	return all[len(all)-2]
}

func TestStatusLinePerProviderProgress(t *testing.T) {
	svc := newMultiService()
	m := checksInFlight(t, svc, 160, 40, provider.GCP, fakeClaude, fakeCodex, fakeSumoPod, fakeOpenRouter)
	line := statusLine(m)
	if !strings.Contains(line, "AWS 0/2 (timeout 8s)") || !strings.Contains(line, m.spinnerFrame()) {
		t.Errorf("while checking: %q", line)
	}
	m, _ = m.dispatch(credsCheckedMsg{seq: m.creds.seq, provider: provider.AWS, report: svc.providerReports[provider.AWS]})
	line = statusLine(m)
	if !strings.Contains(line, "✓ AWS 2/2") || !strings.Contains(line, "Azure 0/1 (timeout 8s)") {
		t.Errorf("after aws: %q", line)
	}
	m, _ = m.dispatch(credsCheckedMsg{seq: m.creds.seq, provider: provider.Azure, report: manager.CredentialReport{
		Warnings: []manager.Warning{{Code: manager.WarnCredentialCheck, Message: "Azure credential check did not finish within 8s"}},
	}})
	if line = statusLine(m); !strings.Contains(line, "? Azure 0/1 unverified") {
		t.Errorf("after azure timeout: %q", line)
	}
	if !strings.Contains(line, "19 accounts · 1 expired · 3 need login · 1 low budget") {
		t.Errorf("totals: %q", line)
	}
	if m.creds.phase != credDone {
		t.Errorf("phase %v after the last provider reported", m.creds.phase)
	}
}

func TestCheckProviderCredentialsPerProviderProgress(t *testing.T) {
	svc := newMultiService()
	m := newTestModel(t, svc, 240, 40) // wide enough that no progress item is collapsed
	m, cmd := m.dispatch(startCredsMsg{})
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("start returned %T, want one command per provider", cmd())
	}
	var msgs []credsCheckedMsg
	for _, c := range batch {
		if msg, ok := c().(credsCheckedMsg); ok {
			msgs = append(msgs, msg)
		}
	}
	var ids []provider.ID
	for _, msg := range msgs {
		ids = append(ids, msg.provider)
	}
	want := []provider.ID{provider.GCP, provider.AWS, provider.Azure, fakeClaude, fakeCodex, fakeSumoPod, fakeOpenRouter}
	if !slices.Equal(ids, want) {
		t.Fatalf("provider messages %v, want %v", ids, want)
	}
	for _, call := range svc.Calls() {
		if call == "CheckCredentials" {
			t.Error("whole-config check issued")
		}
	}
	// Each message updates only its provider.
	for i, msg := range msgs {
		m, _ = m.dispatch(msg)
		line := statusLine(m)
		label := m.providerInfo(msg.provider).Label
		if !strings.Contains(line, "✓ "+label) {
			t.Errorf("after %s: %q", label, line)
		}
		if i < len(msgs)-1 && m.creds.phase != credChecking {
			t.Errorf("done after %d of %d providers", i+1, len(msgs))
		}
	}
	if m.creds.phase != credDone {
		t.Errorf("phase %v after every provider", m.creds.phase)
	}
}

func TestUnverifiedRowShowsAge(t *testing.T) {
	svc := newMultiService()
	for i, a := range svc.overview.Accounts {
		if a.Name == "corp-azure" {
			svc.overview.Accounts[i].Status = manager.HealthUnverified
			svc.overview.Accounts[i].LastGoodAge = 3 * time.Hour
			svc.overview.Accounts[i].Checked = fixtureNow.Add(-time.Minute)
		}
	}
	m := multiModel(t, svc, 160, 40)
	if row := lineWith(t, screen(m), "corp-azure"); !strings.Contains(row, "? UNVERIFIED 3h") {
		t.Errorf("row %q", row)
	}
}

func TestNeedsActionFilter(t *testing.T) {
	m := multiModel(t, newMultiService(), 160, 40)
	m = press(t, m, "!")
	if !m.needsAction {
		t.Fatal("needsAction off")
	}
	want := []string{"byoip-lab", "sbx-aliz", "legacy-aws", "claude-personal", "sumopod-prod"}
	if !slices.Equal(visibleNames(m), want) {
		t.Errorf("needs action %v", visibleNames(m))
	}
	m = press(t, m, "!")
	if len(m.visible) != 19 {
		t.Errorf("after toggling off: %d", len(m.visible))
	}
}

func gcpOrder(m Model) []string {
	var names []string
	for _, idx := range m.visible {
		if a := m.overview.Accounts[idx]; a.Provider == provider.GCP {
			names = append(names, a.Name)
		}
	}
	return names
}

func TestSortCycle(t *testing.T) {
	m := multiModel(t, newMultiService(), 160, 40)
	if m.sort != sortName || gcpOrder(m)[0] != "byoip-lab" {
		t.Fatalf("default sort %v %v", m.sort, gcpOrder(m))
	}
	m = press(t, m, "o")
	if got := gcpOrder(m); m.sort != sortStatus || got[0] != "sbx-aliz" || got[1] != "byoip-lab" {
		t.Errorf("status sort %v", got)
	}
	if !strings.Contains(tabRow(t, m), "sort status") {
		t.Errorf("tab row %q", tabRow(t, m))
	}
	m = press(t, m, "o")
	// Expired first, then soonest expiry; no expiry last.
	if got := gcpOrder(m); m.sort != sortExpiry || got[0] != "byoip-lab" || got[len(got)-1] != "sbx-aliz" {
		t.Errorf("expiry sort %v", got)
	}
	m = press(t, m, "o")
	if m.sort != sortName {
		t.Errorf("sort did not cycle back: %v", m.sort)
	}
}

func TestHintLineFitsWidth(t *testing.T) {
	for _, width := range []int{60, 70, 90, 120, 160} {
		m := multiModel(t, newMultiService(), width, 24)
		out := screen(m)
		all := lines(out)
		if len(all) != 24 {
			t.Errorf("%d: %d lines, want 24", width, len(all))
		}
		for i, line := range all {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("%d: line %d is %d wide: %q", width, i, w, line)
			}
		}
		hints := all[len(all)-1]
		if width == 70 {
			if n := strings.Count(hints, "  ") + 1; n > 5 {
				t.Errorf("70: %d hints: %q", n, hints)
			}
		}
		if !strings.Contains(hints, "activate") {
			t.Errorf("%d: hints %q", width, hints)
		}
	}
}

func TestServiceProvidersMetadataRendersBadges(t *testing.T) {
	m := multiModel(t, newMultiService(), 120, 40)
	out := screen(m)
	for _, row := range [][]string{{" GCP ", "cloudsuite"}, {" AZR ", "corp-azure"}, {"[CLD]", "claude-work"}, {"[OAI]", "codex-work"}, {"[SMP]", "sumopod-dev"}, {"[ORT]", "openrouter"}} {
		if !hasLineWith(out, row...) {
			t.Errorf("no badge %q on %s", row[0], row[1])
		}
	}
	m, _ = m.dispatch(tea.ColorProfileMsg{Profile: colorprofile.ASCII}) // NO_COLOR
	if !hasLineWith(screen(m), "[GCP]", "cloudsuite") {
		t.Error("no-colour badge lacks brackets")
	}
	// Narrow: the badge folds into NAME.
	m = resize(t, m, 90, 30)
	if head := columnHeaderLine(t, screen(m)); strings.Contains(head, "KIND") {
		t.Errorf("narrow header %q", head)
	}
	if !hasLineWith(screen(m), "[GCP] cloudsuite") {
		t.Errorf("narrow name has no badge:\n%s", screen(m))
	}
}

// TestNoProviderSpecificBranchesInTUI keeps pkg/tui driven by provider
// metadata: no provider names in non-test sources.
func TestNoProviderSpecificBranchesInTUI(t *testing.T) {
	cmd := exec.Command("sh", "-c",
		`grep -n -i -E 'claude|codex|sumopod|openrouter|tencent|"(gcp|aws|azure)"|provider\.(GCP|AWS|Azure|Tencent)\b' $(ls *.go | grep -v '_test\.go$')`)
	cmd.Dir = "."
	out, err := cmd.Output()
	if err != nil {
		return // grep exits 1 when nothing matches
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		t.Errorf("provider-specific code in pkg/tui: %s", line)
	}
}

func TestFoldAllReadyGroupCanBeExpanded(t *testing.T) {
	m := multiModel(t, newMultiService(), 160, 40)
	m.selected = "openrouter" // its group has only READY rows
	m = press(t, m, "z")
	key := foldKey(fakeOpenRouter)
	if !m.folded[string(fakeOpenRouter)] || m.selected != key {
		t.Fatalf("after fold: folded %v selected %q", m.folded, m.selected)
	}
	if slices.Contains(visibleNames(m), "openrouter") {
		t.Error("folded row still listed")
	}
	out := screen(m)
	if line := lineWith(t, out, "1 more ready"); !strings.Contains(line, "▌") {
		t.Errorf("placeholder not drawn as selected: %q", line)
	}
	if !strings.Contains(out, "OPENROUTER") || !strings.Contains(out, "1 · 1 ready") {
		t.Errorf("pane or heading lacks the group summary:\n%s", out)
	}
	// Actions do not act on the placeholder.
	before := len(m.svcCalls())
	for _, k := range []string{"enter", "l", "s", "S", "y", "r", "a", "ctrl+d"} {
		m2, _ := m.dispatch(keyMsg(k)) // dispatch: keep the info message, which drain would expire
		if !strings.Contains(m2.status.text, "press z") || m2.modal != nil || len(m.svcCalls()) != before {
			t.Errorf("%s on placeholder: status %q modal %v calls %v", k, m2.status.text, m2.modal, m.svcCalls()[before:])
		}
	}
	// j/k reach it: up then down returns to it.
	m = press(t, m, "k", "j")
	if m.selected != key {
		t.Errorf("placeholder not reachable with j: %q", m.selected)
	}
	m = press(t, m, "z")
	if m.folded[string(fakeOpenRouter)] || m.selected != "openrouter" || len(m.visible) != 19 {
		t.Errorf("after unfold: folded %v selected %q visible %d", m.folded, m.selected, len(m.visible))
	}
	if !strings.Contains(statusLine(m), "19 accounts") {
		t.Errorf("totals: %q", statusLine(m))
	}
}

func TestFoldOnSingleProviderTab(t *testing.T) {
	m := multiModel(t, newMultiService(), 120, 30)
	m = press(t, m, "2") // GCP tab
	m.selected = "cloudsuite"
	m = press(t, m, "z")
	if !m.folded[string(provider.GCP)] || len(m.visible) != 2 || m.selected != foldKey(provider.GCP) {
		t.Fatalf("tab fold: folded %v visible %v selected %q", m.folded, visibleNames(m), m.selected)
	}
	if !hasLineWith(screen(m), "7 more ready") {
		t.Errorf("no placeholder:\n%s", screen(m))
	}
	m = press(t, m, "z")
	if len(m.visible) != 9 || m.selected != "byoip-lab" {
		t.Errorf("tab unfold: visible %d selected %q", len(m.visible), m.selected)
	}
}

// gatingOverview has one GCP and one AWS account with the given CLIs found.
func gatingOverview(gcloud, aws bool) manager.Overview {
	ov := multiOverview()
	var accounts []manager.AccountStatus
	for _, a := range ov.Accounts {
		if a.Name == "cloudsuite" || a.Name == "prod-aws" {
			accounts = append(accounts, a)
		}
	}
	ov.Accounts = accounts
	ov.GcloudAvailable = gcloud
	for i := range ov.Providers {
		ov.Providers[i].Count = 0
		switch ov.Providers[i].ID {
		case provider.GCP:
			ov.Providers[i].Count, ov.Providers[i].CLI = 1, provider.CLIStatus{Name: "gcloud", Found: gcloud}
		case provider.AWS:
			ov.Providers[i].Count, ov.Providers[i].CLI = 1, provider.CLIStatus{Name: "aws", Found: aws}
		}
	}
	return ov
}

func TestMissingGcloudDoesNotBlockAWSRow(t *testing.T) {
	m := newTestModel(t, newFakeService(gatingOverview(false, true)), 120, 30)
	m.selected = "prod-aws"
	for _, k := range []string{"enter", "l", "r"} {
		m2, _ := m.dispatch(keyMsg(k))
		if strings.Contains(m2.status.text, "not found") {
			t.Errorf("%s on AWS row blocked: %q", k, m2.status.text)
		}
	}
}

func TestMissingAWSCLIBlocksOnlyAWSRows(t *testing.T) {
	m := newTestModel(t, newFakeService(gatingOverview(true, false)), 120, 30)
	m.selected = "prod-aws"
	for _, k := range []string{"enter", "l", "r"} {
		m2, _ := m.dispatch(keyMsg(k))
		if !strings.Contains(m2.status.text, "aws not found") {
			t.Errorf("%s on AWS row: %q", k, m2.status.text)
		}
	}
	m.selected = "cloudsuite"
	for _, k := range []string{"enter", "l", "r"} {
		m2, _ := m.dispatch(keyMsg(k))
		if strings.Contains(m2.status.text, "not found") {
			t.Errorf("%s on GCP row blocked: %q", k, m2.status.text)
		}
	}
}

func TestSaveKeyIgnoredOnNonGCPRow(t *testing.T) {
	m := newTestModel(t, newFakeService(gatingOverview(true, true)), 120, 30)
	m.selected = "prod-aws"
	m2, _ := m.dispatch(keyMsg("a"))
	if m2.modal != nil {
		t.Error("a opened the save dialog on an AWS row")
	}
	m.selected = "cloudsuite"
	if m2, _ = m.dispatch(keyMsg("a")); m2.modal == nil {
		t.Error("a did not open the save dialog on a GCP row")
	}
}
