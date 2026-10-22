package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

const fakeTencent provider.ID = "tencent"

// tencentInfo is a registered cloud whose CLI is not installed.
func tencentInfo() manager.ProviderInfo {
	return manager.ProviderInfo{
		ID: fakeTencent, Label: "Tencent", Category: provider.CategoryCloud, Slot: "tencent", TargetLabel: "ACCOUNT",
		Badge:       provider.Badge{Text: "TCT", Style: provider.BadgeFilled, Color: "#c39bff"},
		CLI:         provider.CLIStatus{Name: "tccli"},
		InstallHint: "install tccli: pipx install tccli, or pip install tccli",
		EnvBased:    true, CheckTimeout: 8 * time.Second, CanLogin: true,
	}
}

// withTencent registers the missing-CLI cloud after Azure, and names the
// CLIs of the clouds the way the real providers do.
func withTencent(svc *fakeService) *fakeService {
	var infos []manager.ProviderInfo
	for _, info := range svc.overview.Providers {
		switch info.ID {
		case provider.GCP:
			info.CLI.Name = "gcloud"
		case provider.AWS:
			info.CLI.Name = "aws"
		case provider.Azure:
			info.CLI.Name = "az"
		case fakeSumoPod, fakeOpenRouter:
			info.CLI = provider.CLIStatus{Found: true} // an HTTP gateway has no CLI
		}
		infos = append(infos, info)
		if info.ID == provider.Azure {
			infos = append(infos, tencentInfo())
		}
	}
	svc.overview.Providers = infos
	return svc
}

func TestHelpGeneratedFromKeyMap(t *testing.T) {
	m := multiModel(t, withTencent(newMultiService()), 160, 40)
	m = press(t, m, "?")
	out := screen(m)
	for _, group := range keys.groups() {
		if !strings.Contains(out, strings.ToLower(group.Title)) {
			t.Errorf("help lacks the %q heading", group.Title)
		}
		for _, binding := range group.Bindings {
			h := binding.Help()
			cell := regexp.MustCompile(`(?m)(^|  )` + regexp.QuoteMeta(h.Key) + ` +` + regexp.QuoteMeta(h.Desc) + `(  |$)`)
			if n := len(cell.FindAllString(out, -1)); n != 1 {
				t.Errorf("binding %q %q appears %d times, want once:\n%s", h.Key, h.Desc, n, out)
			}
		}
	}
	if m.screen != screenHelp {
		t.Fatalf("screen %v", m.screen)
	}
	for _, closer := range []string{"?", "esc"} {
		if closed := press(t, m, closer); closed.screen != screenMain {
			t.Errorf("%s did not close help", closer)
		}
	}
}

func TestHelpScrollsWhenTheScreenIsShort(t *testing.T) {
	m := multiModel(t, withTencent(newMultiService()), 70, 24)
	m = press(t, m, "?")
	first := screen(m)
	if strings.Contains(first, "OpenRouter") {
		t.Fatalf("the providers block cannot fit in 70x24:\n%s", first)
	}
	m = press(t, m, "G")
	if last := screen(m); !strings.Contains(last, "OpenRouter") || last == first {
		t.Errorf("G does not scroll to the end:\n%s", last)
	}
	m = press(t, m, "g")
	if screen(m) != first {
		t.Error("g does not return to the top")
	}
}

func TestHelpProvidersBlock(t *testing.T) {
	svc := withTencent(newMultiService())
	svc.cliVersions = map[provider.ID]string{
		provider.GCP:   "Google Cloud SDK 540.0.0",
		provider.AWS:   "aws-cli/2.27.4 Python/3.13.4 Darwin/25.0.0",
		provider.Azure: "azure-cli                         2.75.0 *",
	}
	m := multiModel(t, svc, 160, 40)
	before := len(svc.Calls())
	m.logPath = "/var/tmp/gctx-test/tui-debug.log"

	m, cmd := m.dispatch(keyMsg("?"))
	if calls := svc.Calls()[before:]; slices.Contains(calls, "CLIVersions") {
		t.Fatalf("opening help ran the version probe on the UI path: %v", calls)
	}
	if cmd == nil {
		t.Fatal("opening help started no version probe")
	}
	out := screen(m)
	if row := lineWith(t, out, "GCP", "gcloud"); !strings.Contains(row, "…") {
		t.Errorf("the version shows %q before it is loaded:\n%s", row, out)
	}

	m = drain(t, m, cmd)
	out = screen(m)
	for needle, version := range map[string]string{"gcloud": "540.0.0", "aws": "2.27.4", "az": "2.75.0"} {
		if row := lineWith(t, out, needle, version); strings.Contains(row, "…") {
			t.Errorf("version still loading: %q", row)
		}
	}
	if row := lineWith(t, out, "Tencent", "tccli not found"); !strings.Contains(row, "pipx install tccli") {
		t.Errorf("missing CLI row lacks the install hint: %q", row)
	}
	for _, want := range []struct{ row, capability string }{
		{"GCP", "global switch"}, {"GCP", "ADC"}, {"AWS", "env: AWS_PROFILE"}, {"Azure", "env: AZURE_CONFIG_DIR"},
		{"SumoPod", "OPENAI_BASE_URL"},
	} {
		if row := lineWith(t, out, want.row, want.capability); row == "" {
			t.Errorf("no %s row with %q", want.row, want.capability)
		}
	}
	if !strings.Contains(out, "config /tmp/gctx/config.json") || !strings.Contains(out, "log /var/tmp/gctx-test/tui-debug.log") {
		t.Errorf("footer lacks the config and log paths:\n%s", out)
	}
	if strings.Contains(out, "doctor") {
		t.Error("help mentions a doctor command, which does not exist")
	}

	m.logPath = ""
	if strings.Contains(screen(m), " log ") {
		t.Error("the log path shows although debug logging is off")
	}

	// A second open reuses the versions instead of probing again.
	m = press(t, m, "esc")
	_, cmd = m.dispatch(keyMsg("?"))
	if cmd != nil {
		t.Error("help probed the CLIs again")
	}
}

// firstRunService is a machine with no accounts and three existing CLI setups.
func firstRunService() *fakeService {
	infos := multiProviders()[:3]
	for i, name := range []string{"gcloud", "aws", "az"} {
		infos[i].CLI.Name = name
	}
	infos = append(infos, tencentInfo())
	svc := newFakeService(manager.Overview{
		ConfigPath: "/tmp/gctx/config.json", ActiveBySlot: map[string]string{}, Providers: infos, GcloudAvailable: true,
	})
	var items []manager.DiscoveredAccount
	add := func(id provider.ID, target string, names ...string) {
		for _, name := range names {
			items = append(items, manager.DiscoveredAccount{Provider: id, Name: name, Target: target + name})
		}
	}
	add(provider.GCP, "proj-", "cloudsuite", "infra-hub-prd", "infra-hub-stg", "gap-dev", "hcp-agents", "rca-poc")
	add(provider.AWS, "", "prod-sso", "legacy", "default")
	add(provider.Azure, "0000-", "corp-subscription", "lab")
	svc.discoverRes = manager.DiscoveryResult{
		Items: items,
		Sources: []manager.DiscoverySource{
			{Provider: provider.GCP, Count: 6, Noun: "gcloud configurations"},
			{Provider: provider.AWS, Count: 3, Noun: "profiles in ~/.aws/config"},
			{Provider: provider.Azure, Count: 2, Noun: "subscriptions in ~/.azure"},
		},
	}
	return svc
}

func firstRunModel(t *testing.T, svc *fakeService, width, height int) Model {
	t.Helper()
	m := newTestModel(t, svc, width, height)
	m.now = func() time.Time { return fixtureNow }
	return drain(t, m, m.Init())
}

func TestFirstRunOffersImport(t *testing.T) {
	svc := firstRunService()
	m := firstRunModel(t, svc, 100, 30)
	out := screen(m)
	for _, want := range []string{
		config.AppName + " · no accounts yet",
		"found existing CLI setups you can import.", "nothing is copied except names.",
		"[x] GCP 6 gcloud configurations", "[x] AWS 3 profiles in ~/.aws/config", "[x] AZR 2 subscriptions in ~/.azure",
		"○ TCT tccli not installed", "Import 11", "Review each", "Start empty",
		"c create", "? keys", "q quit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("first run lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[x] TCT") || strings.Contains(out, "[ ] TCT") {
		t.Errorf("a provider without its CLI must not be selectable:\n%s", out)
	}

	// Unchecking a source updates the count; Import creates the checked ones.
	svc.onImport = func(items []manager.DiscoveredAccount) {
		for _, item := range items {
			svc.overview.Accounts = append(svc.overview.Accounts, manager.AccountStatus{
				Name: item.Name, Provider: item.Provider, ProjectID: item.Target, Status: manager.HealthAuthRequired,
			})
		}
	}
	svc.importRes = manager.ImportResult{Imported: make([]string, 9)}
	m = press(t, m, "up", "space")
	if out = screen(m); !strings.Contains(out, "[ ] AZR") || !strings.Contains(out, "Import 9") {
		t.Fatalf("unchecking AZR:\n%s", out)
	}
	m = pressQuick(t, press(t, m, "down"), "enter")
	if len(svc.imported) != 1 || len(svc.imported[0]) != 9 {
		t.Fatalf("imported %v, want the 9 GCP and AWS items", svc.imported)
	}
	for _, item := range svc.imported[0] {
		if item.Provider == provider.Azure {
			t.Errorf("imported unchecked %s", item.Name)
		}
	}
	if m.firstRun != nil || len(m.overview.Accounts) != 9 {
		t.Errorf("after import: firstRun %v, %d accounts", m.firstRun, len(m.overview.Accounts))
	}
	if !strings.Contains(m.status.text, "imported 9") {
		t.Errorf("status %q", m.status.text)
	}
}

func TestFirstRunReviewEachImportsTheChecked(t *testing.T) {
	svc := firstRunService()
	m := firstRunModel(t, svc, 100, 40)
	m = press(t, m, "down", "enter") // focus starts on Import; Review each is next
	out := screen(m)
	for _, want := range []string{"cloudsuite", "proj-cloudsuite", "prod-sso", "corp-subscription", "0000-lab"} {
		if !strings.Contains(out, want) {
			t.Errorf("review lacks %q:\n%s", want, out)
		}
	}
	m = press(t, m, "space") // uncheck the first item, cloudsuite
	m = tabUntil(t, m, "[ Import 10 ]")
	svc.importRes = manager.ImportResult{Imported: make([]string, 10)}
	pressQuick(t, m, "enter")
	if len(svc.imported) != 1 || len(svc.imported[0]) != 10 {
		t.Fatalf("imported %v", svc.imported)
	}
	for _, item := range svc.imported[0] {
		if item.Name == "cloudsuite" {
			t.Error("imported the unchecked item")
		}
	}
}

func TestFirstRunStartEmptyIsNotShownAgain(t *testing.T) {
	svc := firstRunService()
	m := firstRunModel(t, svc, 100, 30)
	m = press(t, m, "down", "down", "enter") // Start empty
	if m.firstRun != nil {
		t.Fatal("still on the first-run screen")
	}
	if out := screen(m); !strings.Contains(out, "No accounts configured. Press c to create one.") {
		t.Errorf("normal empty list expected:\n%s", out)
	}
	m, _ = m.dispatch(tea.FocusMsg{})
	m = drain(t, m, m.Init())
	if m.firstRun != nil {
		t.Error("first run came back in the same session")
	}
	if len(svc.imported) != 0 {
		t.Error("start empty imported accounts")
	}
}

func TestFirstRunNeedsNothingToImportAndNoLoadError(t *testing.T) {
	svc := firstRunService()
	svc.discoverRes = manager.DiscoveryResult{}
	if m := firstRunModel(t, svc, 100, 30); m.firstRun != nil {
		t.Error("first run shown with nothing to import")
	}

	svc = firstRunService()
	svc.overviewErr = fmt.Errorf("config.json: unreadable")
	m := firstRunModel(t, svc, 100, 30)
	if m.firstRun != nil || !strings.Contains(screen(m), "Error loading accounts") {
		t.Errorf("a load error must show the error, not the first run:\n%s", screen(m))
	}
	if slices.Contains(svc.Calls(), "Discover") {
		t.Error("discovery ran although the config could not be loaded")
	}

	svc = firstRunService()
	svc.overview = sampleOverview("a", "a")
	m = firstRunModel(t, svc, 100, 30)
	if m.firstRun != nil || slices.Contains(svc.Calls(), "Discover") {
		t.Error("first run for a machine that has accounts")
	}
}

func TestFirstRunSkipsSourcesWithoutTheirCLI(t *testing.T) {
	svc := firstRunService()
	for i, info := range svc.overview.Providers {
		if info.ID == provider.GCP {
			svc.overview.Providers[i].CLI = provider.CLIStatus{Name: "gcloud"}
		}
	}
	m := firstRunModel(t, svc, 100, 30)
	out := screen(m)
	if !strings.Contains(out, "○ GCP gcloud not installed") || !strings.Contains(out, "Import 5") {
		t.Errorf("gcloud missing:\n%s", out)
	}
	svc.importRes = manager.ImportResult{Imported: make([]string, 5)}
	pressQuick(t, m, "enter")
	for _, item := range svc.imported[0] {
		if item.Provider == provider.GCP {
			t.Errorf("imported %s although gcloud is missing", item.Name)
		}
	}
}

func TestFirstRunKeys(t *testing.T) {
	m := firstRunModel(t, firstRunService(), 100, 30)
	if _, cmd := m.dispatch(keyMsg("q")); cmd == nil {
		t.Error("q did not quit")
	}
	if opened := press(t, m, "?"); opened.screen != screenHelp {
		t.Error("? did not open help")
	}
	if created := press(t, m, "c"); created.modal == nil || created.firstRun != nil {
		t.Errorf("c: modal %T firstRun %v", created.modal, created.firstRun)
	}
}

func TestMissingCLITabShowsInstallHint(t *testing.T) {
	svc := withTencent(newMultiService())
	m := multiModel(t, svc, 120, 30)
	tabs := lineWith(t, screen(m), "All", "GCP", "AWS")
	if !strings.Contains(tabs, "Tencent n/a") {
		t.Errorf("tab row lacks the missing-CLI tab: %q", tabs)
	}
	if strings.Contains(lineWith(t, screen(m), "GCP", "cloudsuite"), "Tencent") {
		t.Error("a provider without accounts must not take a header slot")
	}

	m = press(t, m, "5")
	out := screen(m)
	for _, want := range []string{
		"tccli is not installed",
		"Tencent accounts need it for login, switching and auth checks. Other clouds are unaffected.",
		"$ pipx install tccli", "then press R to rescan PATH",
		"y copy", "R rescan", "tab next provider",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing-CLI tab lacks %q:\n%s", want, out)
		}
	}
	if indent := len(lineWith(t, out, "tccli is not installed")) - len(strings.TrimLeft(lineWith(t, out, "tccli is not installed"), " ")); indent < 10 {
		t.Errorf("explanation is not centred (indent %d):\n%s", indent, out)
	}
	if strings.Contains(out, "No accounts configured") {
		t.Errorf("the empty-list text shows instead of the explanation:\n%s", out)
	}

	cycled := press(t, multiModel(t, svc, 120, 30), "tab", "tab", "tab", "tab")
	if cycled.tab != 4 {
		t.Errorf("tab cycling skipped the missing-CLI tab: tab %d", cycled.tab)
	}

	copied, cmd := m.dispatch(keyMsg("y"))
	if cmd == nil || !strings.Contains(copied.status.text, "copied: pipx install tccli") {
		t.Errorf("y status %q", copied.status.text)
	}
}

func TestRescanKeyRelocatesCLI(t *testing.T) {
	installed := false
	dir := t.TempDir()
	mgr, err := manager.NewWithOptions(manager.Options{
		Paths:  manager.Paths{GctxDir: filepath.Join(dir, "gctx"), GcloudDir: filepath.Join(dir, "gcloud")},
		Home:   dir,
		Getenv: func(string) string { return "" },
		LookPath: func(name string) (string, error) {
			if name == "aws" && !installed {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/" + name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, mgr, 120, 30)
	m = drain(t, m, m.Init())
	m = press(t, m, "tab")
	if out := screen(m); !strings.Contains(out, "aws is not installed") || !strings.Contains(out, "AWS n/a") {
		t.Fatalf("aws tab before the install:\n%s", out)
	}

	m = press(t, m, "R")
	if !strings.Contains(screen(m), "aws is not installed") {
		t.Error("R must keep the explanation while the CLI is still missing")
	}

	installed = true
	m = press(t, m, "R")
	out := screen(m)
	if strings.Contains(out, "aws is not installed") || strings.Contains(out, "AWS n/a") {
		t.Errorf("R did not re-locate the CLI:\n%s", out)
	}
	if m.tab != 0 {
		t.Errorf("the tab of a provider with no accounts must close once its CLI is found: tab %d", m.tab)
	}
}

// markOffline gives every READY account the UNVERIFIED health a failed check
// leaves, with the age of its last good result.
func markOffline(svc *fakeService) {
	ages := []time.Duration{2 * time.Hour, 3 * 24 * time.Hour, 2 * time.Hour}
	n := 0
	for i, a := range svc.overview.Accounts {
		if a.Status != manager.HealthReady {
			continue
		}
		svc.overview.Accounts[i].Status = manager.HealthUnverified
		svc.overview.Accounts[i].Findings = []manager.Finding{{Health: manager.HealthUnverified}}
		svc.overview.Accounts[i].LastGoodAge = ages[n%len(ages)]
		n++
	}
}

// rowOf is the last line naming an account: the header slots come first.
func rowOf(out, name string) string {
	row := ""
	for _, line := range lines(out) {
		if strings.Contains(line, name) {
			row = line
		}
	}
	return row
}

var timedOut = manager.CredentialReport{Warnings: []manager.Warning{{Code: manager.WarnCredentialCheck, Message: "check timed out"}}}

// finishChecks delivers one report per listed provider; reports[id] defaults
// to the timeout.
func finishChecks(m Model, reports map[provider.ID]manager.CredentialReport) Model {
	for _, info := range m.listedProviders() {
		if info.Count == 0 {
			continue
		}
		report, ok := reports[info.ID]
		if !ok {
			report = timedOut
		}
		m, _ = m.dispatch(credsCheckedMsg{seq: m.creds.seq, provider: info.ID, report: report})
	}
	return m
}

func TestOfflineBannerAndUnverifiedRows(t *testing.T) {
	svc := newMultiService()
	markOffline(svc)
	m := checksInFlight(t, svc, 120, 40)
	if strings.Contains(screen(m), "network unreachable") {
		t.Fatal("banner while the checks are still running")
	}
	m = finishChecks(m, nil)
	out := screen(m)
	banner := "⚠ network unreachable · showing last known status"
	if !strings.Contains(out, banner) {
		t.Fatalf("no offline banner:\n%s", out)
	}
	if bannerAt, rowsAt := strings.Index(out, banner), strings.Index(out, "TARGET"); bannerAt > rowsAt {
		t.Errorf("the banner must sit above the table:\n%s", out)
	}
	for _, want := range []string{"R retry", "switching still works offline"} {
		if !strings.Contains(out, want) {
			t.Errorf("offline screen lacks %q:\n%s", want, out)
		}
	}
	if row := rowOf(out, "prod-aws"); !regexp.MustCompile(`UNVERIFIED \d+[hd]`).MatchString(row) || strings.Contains(row, "AUTH REQUIRED") {
		t.Errorf("prod-aws: %q", row)
	}
	if row := rowOf(out, "legacy-aws"); !strings.Contains(row, "AUTH REQUIRED") {
		t.Errorf("local findings still show: %q", row)
	}

	// Retrying: one check that succeeds ends the offline state.
	m, _ = m.dispatch(startCredsMsg{})
	m, _ = m.dispatch(credsCheckedMsg{seq: m.creds.seq, provider: provider.AWS, report: manager.CredentialReport{Accounts: []string{"prod-aws"}}})
	if strings.Contains(screen(m), "network unreachable") {
		t.Errorf("banner survived a successful check:\n%s", screen(m))
	}
}

func TestSingleSlowProviderIsNotOffline(t *testing.T) {
	svc := newMultiService()
	markOffline(svc)
	m := checksInFlight(t, svc, 120, 40)
	reports := multiReports()
	reports[provider.GCP] = timedOut
	m = finishChecks(m, reports)
	out := screen(m)
	if strings.Contains(out, "network unreachable") || strings.Contains(out, "switching still works offline") {
		t.Errorf("one slow provider is not an outage:\n%s", out)
	}
	if !strings.Contains(out, "UNVERIFIED") {
		t.Errorf("the slow provider's rows must still show UNVERIFIED:\n%s", out)
	}
}

func TestOfflineBannerNeedsEveryCheckToFail(t *testing.T) {
	svc := newMultiService()
	m := checksInFlight(t, svc, 120, 40)
	// A hard failure of the check itself counts as unreachable too.
	for _, info := range m.listedProviders() {
		m, _ = m.dispatch(credsCheckedMsg{seq: m.creds.seq, provider: info.ID, err: fmt.Errorf("dial tcp: lookup failed")})
	}
	if !strings.Contains(screen(m), "network unreachable") {
		t.Errorf("failed checks must show the banner:\n%s", screen(m))
	}
}

// TestNoProviderLiteralsInTUI keeps provider names out of the TUI: it works
// from provider metadata only.
func TestNoProviderLiteralsInTUI(t *testing.T) {
	forbidden := []string{
		"provider.GCP", "provider.AWS", "provider.Azure",
		`"gcp"`, `"aws"`, `"azure"`, `"tencent"`, `"claude"`, `"codex"`, `"sumopod"`, `"openrouter"`, `"openai-api"`,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, literal := range forbidden {
			if strings.Contains(string(source), literal) {
				t.Errorf("%s names a provider: %s", file, literal)
			}
		}
	}
}
