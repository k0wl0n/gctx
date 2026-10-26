package tui

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// managerFor builds a real manager over the real providers, with the named
// CLIs reported missing.
func managerFor(t *testing.T, missing ...string) (*manager.Manager, manager.Paths) {
	t.Helper()
	dir := t.TempDir()
	paths := manager.Paths{
		GctxDir:    filepath.Join(dir, "gctx"),
		ADCDir:     filepath.Join(dir, "gctx", "adc"),
		GcloudDir:  filepath.Join(dir, "gcloud"),
		DefaultADC: filepath.Join(dir, "gcloud", "application_default_credentials.json"),
	}
	m, err := manager.NewWithOptions(manager.Options{
		Paths:  paths,
		Getenv: func(string) string { return "" },
		LookPath: func(name string) (string, error) {
			if slices.Contains(missing, name) {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/" + name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, paths
}

// formService is a fake service over the real providers' metadata with one GCP
// account (alice) and one active AWS account (prod-aws).
func formService(t *testing.T, missing ...string) *fakeService {
	t.Helper()
	mgr, _ := managerFor(t, missing...)
	overview := sampleOverview("alice", "alice")
	overview.Providers = mgr.Providers()
	overview.Accounts = append(overview.Accounts, manager.AccountStatus{
		Name: "prod-aws", Provider: provider.AWS, Category: provider.CategoryCloud, TargetLabel: "PROFILE",
		ProjectID: "prod-sso", Active: true, Status: manager.HealthReady,
		Settings: map[string]string{"auth": "sso", "region": "eu-west-1", "sso_start_url": "https://corp.awsapps.com/start"},
	})
	overview.ActiveBySlot["aws"] = "prod-aws"
	return newFakeService(overview)
}

// pressQuick is press without waiting for timer commands such as the status
// expiry tick: a command that takes longer than the budget is dropped.
func pressQuick(t *testing.T, m Model, name string) Model {
	t.Helper()
	m, cmd := m.dispatch(keyMsg(name))
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0 && steps < drainLimit; steps++ {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- next() }()
		var msg tea.Msg
		select {
		case msg = <-done:
		case <-time.After(200 * time.Millisecond):
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if msg == nil {
			continue
		}
		var follow tea.Cmd
		m, follow = m.dispatch(msg)
		queue = append(queue, follow)
	}
	return m
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = press(t, m, string(r))
	}
	return m
}

// focusedButton names the focused form button as "[*label*]", the marker the
// no-colour profile draws; colour profiles fill the button instead.
func focusedButton(m Model) string {
	form, ok := m.modal.(*accountForm)
	if !ok {
		return ""
	}
	info, _ := form.info()
	switch form.focused().kind {
	case itemLogin:
		return "[*Create and login*]"
	case itemSubmit:
		switch {
		case form.editing != nil:
			return "[*Save*]"
		case info.CanLogin:
			return "[*Create only*]"
		}
		return "[*Create*]"
	case itemCancel:
		return "[*Cancel*]"
	}
	return ""
}

// flatForm joins the modal's lines without borders, so text the box wraps
// at a hyphen or space reads as one line again.
func flatForm(out string) string {
	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte(' ')
		}
		b.WriteString(line)
	}
	return b.String()
}

// tabUntil presses tab until the screen shows marker or the marker names
// the focused button.
func tabUntil(t *testing.T, m Model, marker string) Model {
	t.Helper()
	for range 40 {
		if strings.Contains(screen(m), marker) || focusedButton(m) == marker {
			return m
		}
		m = press(t, m, "tab")
	}
	t.Fatalf("never reached %q:\n%s", marker, screen(m))
	return m
}

func openForm(t *testing.T, svc Service) Model {
	t.Helper()
	m := newTestModel(t, svc, 120, 40)
	m = press(t, m, "c")
	if m.modal == nil {
		t.Fatal("c did not open the form")
	}
	return m
}

// openAWS opens the form with AWS selected and focus on Name.
func openAWS(t *testing.T, svc Service) Model {
	t.Helper()
	return press(t, openForm(t, svc), "right", "tab")
}

func lineIndex(t *testing.T, text, containing string) int {
	t.Helper()
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, containing) {
			return i
		}
	}
	t.Fatalf("no line containing %q:\n%s", containing, text)
	return -1
}

func TestKindListsOnlyRegisteredCategories(t *testing.T) {
	out := screen(openForm(t, formService(t)))
	if !strings.Contains(out, "Cloud") {
		t.Errorf("Cloud kind missing:\n%s", out)
	}
	for _, kind := range []string{"AI tool", "AI gateway"} {
		if strings.Contains(out, kind) {
			t.Errorf("%s listed without a registered provider:\n%s", kind, out)
		}
	}

	out = screen(openForm(t, newMultiService()))
	for _, kind := range []string{"Cloud", "AI tool", "AI gateway"} {
		if !strings.Contains(out, kind) {
			t.Errorf("%s missing with providers registered:\n%s", kind, out)
		}
	}
}

func TestProviderSwitchRebuildsFields(t *testing.T) {
	m := openForm(t, formService(t))
	out := screen(m)
	if !strings.Contains(out, "Project ID") || strings.Contains(out, "SSO start URL") {
		t.Fatalf("GCP form wrong:\n%s", out)
	}

	m = press(t, m, "right")
	out = screen(m)
	if !strings.Contains(out, "SSO start URL") || !strings.Contains(out, "Profile name") || strings.Contains(out, "Project ID") {
		t.Fatalf("AWS form wrong:\n%s", out)
	}

	m = tabUntil(t, m, "› Auth")
	m = press(t, m, "right") // sso -> profile
	if out = screen(m); strings.Contains(out, "SSO start URL") || strings.Contains(out, "SSO region") {
		t.Errorf("auth=profile must hide the SSO fields:\n%s", out)
	}
	m = press(t, m, "left")
	if out = screen(m); !strings.Contains(out, "SSO start URL") {
		t.Errorf("auth=sso must show the SSO fields again:\n%s", out)
	}
}

func TestNameUniqueAcrossProviders(t *testing.T) {
	m := press(t, openForm(t, formService(t)), "tab")
	m = typeText(t, m, "prod-aws")
	out := screen(m)
	if !strings.Contains(out, "✗") || !strings.Contains(out, "already") || !strings.Contains(out, "AWS") {
		t.Errorf("duplicate across providers not reported:\n%s", out)
	}

	m = press(t, m, "ctrl+u")
	m = typeText(t, m, "fresh-name")
	if out = screen(m); !strings.Contains(out, "✓ unique across all accounts") {
		t.Errorf("unique name not confirmed:\n%s", out)
	}

	m = press(t, m, "ctrl+u")
	m = typeText(t, m, "Bad Name")
	if out = screen(m); !strings.Contains(out, "✗") || !strings.Contains(out, "lowercase") {
		t.Errorf("invalid name not reported:\n%s", out)
	}
}

func TestFieldValidationInline(t *testing.T) {
	svc := formService(t)
	m := openAWS(t, svc)
	m = typeText(t, m, "good-name")
	m = tabUntil(t, m, "› Profile name")
	m = typeText(t, m, "prod eu data")

	out := screen(m)
	field := lineIndex(t, out, "Profile name")
	lines := strings.Split(out, "\n")
	under := lines[field+1] + lines[field+2] // the input's underline sits between
	if !strings.Contains(under, "✗") || !strings.Contains(under, "invalid AWS profile name") {
		t.Fatalf("error not directly under the field:\n%s", out)
	}

	m = tabUntil(t, m, "[*Create only*]")
	m = press(t, m, "enter")
	if m.modal == nil || len(svc.createReqs) != 0 || m.op != nil {
		t.Errorf("submit must be blocked by the invalid field: modal=%v reqs=%v", m.modal != nil, svc.createReqs)
	}
}

func TestCreateDisabledWhenCLIMissing(t *testing.T) {
	svc := formService(t, "aws", "gcloud")
	m := newTestModel(t, svc, 120, 40)
	m = press(t, m, "c")
	if m.modal == nil {
		t.Fatal("c must open the form even when the CLI is missing")
	}
	if out := screen(m); !strings.Contains(out, "cloud.google.com/sdk/docs/install") {
		t.Errorf("gcloud install hint missing:\n%s", out)
	}

	m = press(t, m, "right")
	out := screen(m)
	if !strings.Contains(out, "brew install awscli") || !strings.Contains(out, "disabled") {
		t.Fatalf("aws install hint or disabled marker missing:\n%s", out)
	}
	m = typeText(t, press(t, m, "tab"), "newacct")
	for range 12 {
		m = press(t, m, "tab")
		if strings.Contains(screen(m), "[*Create") {
			t.Fatalf("a disabled Create button took focus:\n%s", screen(m))
		}
	}
	m = press(t, m, "enter")
	if len(svc.createReqs) != 0 {
		t.Error("disabled form created an account")
	}
}

func TestFooterListsWritesAndNever(t *testing.T) {
	m := openAWS(t, formService(t))
	m = typeText(t, m, "prod-eu-data")
	m = tabUntil(t, m, "› Profile name")
	m = typeText(t, m, "prod-eu-data")
	out := screen(m)
	for _, want := range []string{"writes", "config.json", "~/.aws/config [profile prod-eu-data]", "never", "access keys", "SSO tokens"} {
		if !strings.Contains(flatForm(out), want) {
			t.Errorf("footer lacks %q:\n%s", want, out)
		}
	}

	m = press(t, m, "shift+tab", "right") // Auth -> profile
	if out = screen(m); strings.Contains(out, "[profile") {
		t.Errorf("adopting a profile writes no ~/.aws/config entry:\n%s", out)
	}

	out = screen(openForm(t, formService(t)))
	if !strings.Contains(out, "writes") || !strings.Contains(out, "never") {
		t.Errorf("gcp footer missing:\n%s", out)
	}
}

// secretProvider is a gateway-like provider with a key-reference field.
func secretService(t *testing.T) *fakeService {
	svc := formService(t)
	svc.overview.Providers = append(svc.overview.Providers, manager.ProviderInfo{
		ID: "fakegw", Label: "FakeGW", Category: provider.CategoryAIGateway, Slot: "openai-api",
		CLI:  provider.CLIStatus{Name: "fakegw", Found: true},
		Form: []provider.FormField{{Key: "key_ref", Label: "Key reference", Secret: true}},
	})
	return svc
}

func TestSecretFieldRejectsRawKey(t *testing.T) {
	svc := secretService(t)
	m := openForm(t, svc)
	m = press(t, m, "shift+tab", "right", "tab", "tab") // Kind -> AI gateway, Provider, Name
	m = typeText(t, m, "gw-one")
	m = tabUntil(t, m, "› Key reference")
	rawKey := "sk-or-v1-0123456789abcdef"
	m = typeText(t, m, rawKey)

	out := screen(m)
	if !strings.Contains(out, "store a reference, not the key") {
		t.Fatalf("raw key accepted:\n%s", out)
	}
	if strings.Contains(out, rawKey) {
		t.Errorf("raw key echoed on screen:\n%s", out)
	}
	m = tabUntil(t, m, "[*Create*]")
	m = press(t, m, "enter")
	if len(svc.createReqs) != 0 {
		t.Fatalf("raw key submitted: %+v", svc.createReqs)
	}

	m = tabUntil(t, m, "› Key reference")
	m = press(t, m, "ctrl+u")
	m = typeText(t, m, "op://Private/gw/credential")
	if out = screen(m); strings.Contains(out, "store a reference") {
		t.Errorf("a reference was refused:\n%s", out)
	}
	m = tabUntil(t, m, "[*Create*]")
	m = pressQuick(t, m, "enter")
	if len(svc.createReqs) != 1 {
		t.Fatalf("reference not submitted: %+v", svc.createReqs)
	}
	req := svc.createReqs[0]
	if req.Provider != "fakegw" || req.Name != "gw-one" || req.Fields["key_ref"] != "op://Private/gw/credential" {
		t.Errorf("request %+v", req)
	}
}

func submitCreate(t *testing.T, svc *fakeService, button string) Model {
	t.Helper()
	m := press(t, openForm(t, svc), "tab")
	m = typeText(t, m, "newacct")
	m = press(t, m, "tab")
	m = typeText(t, m, "new-project-1")
	m.exec = execSync("", nil)
	m = tabUntil(t, m, button)

	// The account exists once the create returns.
	svc.createRes = manager.CreateResult{Account: "newacct"}
	created := sampleOverview("alice", "alice", "newacct")
	created.Providers = svc.overview.Providers
	svc.overview = created
	return pressQuick(t, m, "enter")
}

func TestCreateAndLoginRunsLoginAfterCreate(t *testing.T) {
	svc := formService(t)
	m := submitCreate(t, svc, "[*Create and login*]")
	calls := svc.Calls()
	create, login := slices.Index(calls, "CreateAccount newacct"), slices.Index(calls, "Login newacct")
	if create < 0 || login < create {
		t.Fatalf("calls %v: want CreateAccount then Login", calls)
	}
	if req := svc.createReqs[0]; req.Provider != provider.GCP || req.Target != "new-project-1" {
		t.Errorf("request %+v", req)
	}
	if m.selected != "newacct" {
		t.Errorf("selected %q, want newacct", m.selected)
	}
}

func TestCreateOnlySkipsLogin(t *testing.T) {
	svc := formService(t)
	m := submitCreate(t, svc, "[*Create only*]")
	calls := svc.Calls()
	if !slices.Contains(calls, "CreateAccount newacct") || slices.Contains(calls, "Login newacct") {
		t.Fatalf("calls %v: want CreateAccount without Login", calls)
	}
	if m.selected != "newacct" || !strings.Contains(m.status.text, "created newacct") {
		t.Errorf("selected %q status %q", m.selected, m.status.text)
	}
	if m.op != nil || m.modal != nil {
		t.Error("create must finish and close the form")
	}
}

func TestCreateFormGCPValidatesProjectID(t *testing.T) {
	svc := formService(t)
	m := press(t, openForm(t, svc), "tab")
	m = typeText(t, m, "newacct")
	m = press(t, m, "tab")
	m = typeText(t, m, "BAD_ID")
	if out := screen(m); !strings.Contains(out, "✗") {
		t.Errorf("project id error not shown:\n%s", out)
	}
	m = tabUntil(t, m, "[*Create only*]")
	m = press(t, m, "enter")
	if m.modal == nil || len(svc.createReqs) != 0 {
		t.Error("invalid project id must block submit")
	}
}

func TestCreateFormBusyBlocksSubmit(t *testing.T) {
	svc := formService(t)
	m := press(t, openForm(t, svc), "tab")
	m = typeText(t, m, "newacct")
	m = press(t, m, "tab")
	m = typeText(t, m, "new-project-1")
	m.op = &operation{kind: opSwitch, account: "alice", desc: "switching", seq: 9}
	m = tabUntil(t, m, "[*Create only*]")
	m = press(t, m, "enter")
	if len(svc.createReqs) != 0 || !strings.Contains(m.status.text, "Busy") {
		t.Errorf("busy submit: reqs=%v status=%q", svc.createReqs, m.status.text)
	}
}

func TestEscCancelsForm(t *testing.T) {
	m := press(t, openForm(t, formService(t)), "esc")
	if m.modal != nil {
		t.Error("esc must close the form")
	}
}

// realModel opens the TUI over a real manager whose config holds one active
// AWS account.
func realModel(t *testing.T) (Model, *manager.Manager) {
	t.Helper()
	mgr, paths := managerFor(t)
	cfg, err := config.LoadFrom(paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	account := &config.Account{
		Name: "prod-aws", ProjectID: "prod-sso", Provider: "aws",
		Settings: map[string]string{"auth": "sso", "region": "eu-west-1", "sso_start_url": "https://corp.awsapps.com/start", "sso_region": "eu-west-1"},
	}
	if err := cfg.AddAccount(account); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetActiveSlot("aws", "prod-aws"); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, mgr, 120, 40)
	m.selected = "prod-aws"
	return m, mgr
}

func TestEditRenameKeepsSlot(t *testing.T) {
	m, _ := realModel(t)
	m = press(t, m, "e")
	if out := screen(m); !strings.Contains(out, "Edit account") || !strings.Contains(out, "prod-aws") {
		t.Fatalf("edit form not prefilled:\n%s", out)
	}
	m = press(t, m, "ctrl+u")
	m = typeText(t, m, "prod-eu")
	m = tabUntil(t, m, "[*Save*]")
	m = pressQuick(t, m, "enter")

	if m.modal != nil {
		t.Fatalf("form still open:\n%s", screen(m))
	}
	renamed, ok := m.overview.Account("prod-eu")
	if _, old := m.overview.Account("prod-aws"); !ok || old {
		t.Fatalf("accounts after rename: %+v", m.overview.Accounts)
	}
	if !renamed.Active || m.overview.ActiveBySlot["aws"] != "prod-eu" {
		t.Errorf("rename lost the slot: active=%v slots=%v", renamed.Active, m.overview.ActiveBySlot)
	}
	if renamed.Settings["region"] != "eu-west-1" || m.selected != "prod-eu" {
		t.Errorf("settings %v selected %q", renamed.Settings, m.selected)
	}
}

func TestEditSavesChangedEditableField(t *testing.T) {
	m, mgr := realModel(t)
	m = press(t, m, "e", "tab") // Name -> Region
	m = press(t, m, "ctrl+u")
	m = typeText(t, m, "us-east-1")
	m = tabUntil(t, m, "[*Save*]")
	pressQuick(t, m, "enter")
	info, err := mgr.Info("prod-aws")
	if err != nil {
		t.Fatal(err)
	}
	if info.Settings["region"] != "us-east-1" {
		t.Errorf("region = %q", info.Settings["region"])
	}
}

func TestEditReadOnlyIdentityFields(t *testing.T) {
	m, _ := realModel(t)
	m = press(t, m, "e")
	out := screen(m)
	for _, want := range []string{"prod-sso", "https://corp.awsapps.com/start", "eu-west-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("identity value %q not shown:\n%s", want, out)
		}
	}
	m = press(t, m, "tab")
	if out = screen(m); !strings.Contains(out, "› Region") {
		t.Fatalf("tab from Name must skip read-only fields and land on Region:\n%s", out)
	}
	for _, key := range []string{"x", "y", "tab", "z"} {
		m = press(t, m, key)
	}
	out = screen(m)
	if !strings.Contains(out, "prod-sso") || strings.Contains(out, "prod-ssoz") || strings.Contains(out, "prod-ssox") {
		t.Errorf("identity field changed:\n%s", out)
	}
}

func TestFormLayoutAt120And70(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 40}, {70, 24}} {
		m := resize(t, openAWS(t, formService(t)), size.width, size.height)
		m = typeText(t, m, "prod-eu-data")
		out := screen(m)
		lines := strings.Split(out, "\n")
		if len(lines) > size.height {
			t.Errorf("%dx%d: %d lines exceed the height", size.width, size.height, len(lines))
		}
		for _, line := range lines {
			if w := len([]rune(line)); w > size.width {
				t.Errorf("%dx%d: line overflows (%d): %q", size.width, size.height, w, line)
			}
		}
		if !strings.Contains(out, "│ Create and login │") || !strings.Contains(out, "│ Create only │") || !strings.Contains(out, "Cancel") {
			t.Errorf("%dx%d: buttons broken:\n%s", size.width, size.height, out)
		}
		if i := lineIndex(t, out, "│ Create and login │"); !strings.Contains(lines[i], "│ Create only │") {
			t.Errorf("%dx%d: buttons wrapped:\n%s", size.width, size.height, out)
		}
	}
}
