package claudecode_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/claudecode"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

var (
	_ provider.Provider        = (*claudecode.Provider)(nil)
	_ provider.EnvBased        = (*claudecode.Provider)(nil)
	_ provider.Linker          = (*claudecode.Provider)(nil)
	_ provider.SetupDiscoverer = (*claudecode.Provider)(nil)
	_ provider.DeleteDescriber = (*claudecode.Provider)(nil)
	_ provider.Claimer         = (*claudecode.Provider)(nil)
)

const (
	fakeToken = "FAKE-CLAUDE-SECRET-TOKEN"
	fakeOrgID = "org-FAKE-ORG-ID"
)

var fakeClaudeBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gctx-fakeclaude-")
	if err != nil {
		panic(err)
	}
	fakeClaudeBin = filepath.Join(dir, "claude")
	if out, err := exec.Command("go", "build", "-o", fakeClaudeBin, "github.com/k0wl0n/gctx/internal/testutil/fakeclaude").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakeclaude: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type harness struct {
	provider *claudecode.Provider
	root     string
	home     string
	logPath  string
}

func newHarness(t *testing.T, extraEnv ...string) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{root: filepath.Join(dir, "gctx", "ai"), home: filepath.Join(dir, "home"), logPath: filepath.Join(dir, "calls.log")}
	h.provider = claudecode.New(claudecode.Options{
		ConfigRoot: h.root,
		Home:       h.home,
		LookPath:   func(string) (string, error) { return fakeClaudeBin, nil },
		Getenv:     func(string) string { return "" },
		Runner:     execrunner.New(fakeClaudeBin).WithEnv(append([]string{"FAKE_CLAUDE_LOG=" + h.logPath}, extraEnv...)),
	})
	return h
}

type call struct {
	Args      []string `json:"args"`
	ConfigDir string   `json:"claude_config_dir"`
}

func (h *harness) calls(t *testing.T) []call {
	t.Helper()
	f, err := os.Open(h.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var all []call
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry call
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		all = append(all, entry)
	}
	return all
}

func (h *harness) created(t *testing.T, name string) config.Account {
	t.Helper()
	acct := config.Account{Name: name, Provider: string(provider.ClaudeCode), Settings: map[string]string{}}
	outcome, err := h.provider.Create(context.Background(), acct)
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(acct.Settings, outcome.Settings)
	return acct
}

func (h *harness) login(t *testing.T, acct config.Account) {
	t.Helper()
	if _, err := h.provider.Login(context.Background(), acct, provider.LoginIO{}); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeMetadata(t *testing.T) {
	h := newHarness(t)
	p := h.provider
	if p.ID() != provider.ClaudeCode || p.Category() != provider.CategoryAITool || p.Slot() != "claude" {
		t.Fatalf("id %s category %s slot %s", p.ID(), p.Category(), p.Slot())
	}
	if badge := p.Badge(); badge.Text != "CLD" || badge.Style != provider.BadgeOutline {
		t.Fatalf("badge %+v", badge)
	}
	if err := p.ValidateTarget(""); err != nil {
		t.Fatalf("empty org label rejected: %v", err)
	}
	if err := p.ValidateTarget("bad\nlabel"); err == nil {
		t.Fatal("control characters accepted in the org label")
	}
}

func TestClaudeEnvIsolatesPerAccount(t *testing.T) {
	h := newHarness(t)
	work, personal := h.created(t, "work"), h.created(t, "personal")

	for _, acct := range []config.Account{work, personal} {
		dir := filepath.Join(h.root, acct.Name)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s dir %v err %v", acct.Name, info, err)
		}
		env, err := h.provider.Env(acct)
		if err != nil || !slices.Equal(env, []string{"CLAUDE_CONFIG_DIR=" + dir}) {
			t.Fatalf("%s env %v err %v", acct.Name, env, err)
		}
	}

	h.login(t, work)
	calls := h.calls(t)
	if len(calls) != 1 || strings.Join(calls[0].Args, " ") != "auth login" || calls[0].ConfigDir != filepath.Join(h.root, "work") {
		t.Fatalf("login calls %+v", calls)
	}

	report := h.provider.CheckAuth(context.Background(), []config.Account{work, personal})
	if len(report.Warnings) != 0 {
		t.Fatalf("warnings %+v", report.Warnings)
	}
	if res := report.Results["work"]; !res.Credentialed || res.Err != nil || res.Identity != "dev@example.com" {
		t.Fatalf("work %+v", res)
	}
	if res := report.Results["personal"]; res.Credentialed || res.Err != nil {
		t.Fatalf("personal must stay logged out: %+v", res)
	}
}

func TestClaudeCheckAuthExpired(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	h.login(t, acct)
	// The CLI forgets the login (expired or revoked): status says loggedIn false, exit 1.
	if err := os.Remove(filepath.Join(h.root, "work", ".credentials.json")); err != nil {
		t.Fatal(err)
	}
	res := h.provider.CheckAuth(context.Background(), []config.Account{acct}).Results["work"]
	if res.Credentialed || res.Err != nil {
		t.Fatalf("logged-out status must be a not-credentialed verdict: %+v", res)
	}

	failing := newHarness(t, "FAKE_CLAUDE_STATUS_FAIL=1")
	acct = failing.created(t, "work")
	res = failing.provider.CheckAuth(context.Background(), []config.Account{acct}).Results["work"]
	if res.Credentialed || res.Err == nil {
		t.Fatalf("an unexpected failure must be unverified: %+v", res)
	}
}

func TestClaudeCheckAuthSkipsLinkedAccounts(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "vertex")
	acct.Settings[provider.SettingVia] = "gcp-prod"
	report := h.provider.CheckAuth(context.Background(), []config.Account{acct})
	if _, ok := report.Results["vertex"]; ok || len(h.calls(t)) != 0 {
		t.Fatalf("a linked account has no own login to check: %+v calls %v", report, h.calls(t))
	}
}

func TestClaudeLinkEnv(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "vertex")
	dir := filepath.Join(h.root, "vertex")

	acct.Settings[provider.SettingVia] = "gcp-prod"
	acct.Settings[provider.SettingBackend] = "vertex"
	acct.Settings[provider.SettingRegion] = "us-east5"
	linked := config.Account{Name: "gcp-prod", ProjectID: "my-project"}
	env, err := h.provider.LinkEnv(acct, linked, provider.GCP)
	want := []string{"CLAUDE_CONFIG_DIR=" + dir, "CLAUDE_CODE_USE_VERTEX=1", "ANTHROPIC_VERTEX_PROJECT_ID=my-project", "CLOUD_ML_REGION=us-east5"}
	if err != nil || !slices.Equal(env, want) {
		t.Fatalf("vertex env %v err %v", env, err)
	}

	acct.Settings[provider.SettingBackend] = "bedrock"
	acct.Settings[provider.SettingRegion] = "us-west-2"
	env, err = h.provider.LinkEnv(acct, config.Account{Name: "aws-prod", ProjectID: "prod", Provider: "aws"}, provider.AWS)
	want = []string{"CLAUDE_CONFIG_DIR=" + dir, "CLAUDE_CODE_USE_BEDROCK=1", "AWS_REGION=us-west-2"}
	if err != nil || !slices.Equal(env, want) {
		t.Fatalf("bedrock env %v err %v", env, err)
	}

	if _, err := h.provider.LinkEnv(acct, linked, provider.GCP); err == nil {
		t.Fatal("bedrock on a GCP account accepted")
	}
	delete(acct.Settings, provider.SettingBackend)
	if _, err := h.provider.LinkEnv(acct, config.Account{Name: "z", Provider: "azure"}, provider.Azure); err == nil || !strings.Contains(err.Error(), "Azure") {
		t.Fatalf("azure link: %v", err)
	}
}

func TestAIToolMissingCLI(t *testing.T) {
	dir := t.TempDir()
	p := claudecode.New(claudecode.Options{
		ConfigRoot: filepath.Join(dir, "ai"),
		LookPath:   func(string) (string, error) { return "", exec.ErrNotFound },
		Getenv:     func(string) string { return "" },
	})
	if cli := p.CLI(); cli.Found || cli.Name != "claude" {
		t.Fatalf("cli %+v", cli)
	}
	acct := config.Account{Name: "work", Provider: "claude"}
	if _, err := p.Login(context.Background(), acct, provider.LoginIO{}); !errors.Is(err, claudecode.ErrCLINotFound) {
		t.Fatalf("login err %v", err)
	}
	report := p.CheckAuth(context.Background(), []config.Account{acct})
	if len(report.Warnings) != 1 || len(report.Results) != 0 {
		t.Fatalf("report %+v", report)
	}
	if hint := p.InstallHint(); !strings.Contains(hint, "claude") {
		t.Fatalf("install hint %q", hint)
	}
}

func TestClaudeDiscoverAdoptsExistingDir(t *testing.T) {
	h := newHarness(t)
	if _, setups, err := h.provider.DiscoverSetups(); err != nil || len(setups) != 0 {
		t.Fatalf("nothing to adopt yet: %v %v", setups, err)
	}
	existing := filepath.Join(h.home, ".claude")
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	location, setups, err := h.provider.DiscoverSetups()
	if err != nil || location != existing || len(setups) != 1 {
		t.Fatalf("location %q setups %+v err %v", location, setups, err)
	}
	setup := setups[0]
	if setup.Name != "claude-default" || setup.Settings[provider.SettingDir] != existing ||
		setup.Settings[provider.SettingConfigOrigin] != provider.ConfigOriginImported {
		t.Fatalf("setup %+v", setup)
	}

	custom := filepath.Join(h.home, "elsewhere")
	if err := os.MkdirAll(custom, 0o700); err != nil {
		t.Fatal(err)
	}
	p := claudecode.New(claudecode.Options{
		ConfigRoot: h.root, Home: h.home,
		LookPath: func(string) (string, error) { return fakeClaudeBin, nil },
		Getenv: func(key string) string {
			if key == "CLAUDE_CONFIG_DIR" {
				return custom
			}
			return ""
		},
	})
	if location, _, _ := p.DiscoverSetups(); location != custom {
		t.Fatalf("CLAUDE_CONFIG_DIR ignored: %q", location)
	}
}

func TestClaudeDeleteNeverRemovesAdoptedDir(t *testing.T) {
	h := newHarness(t)
	existing := filepath.Join(h.home, ".claude")
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	_, setups, _ := h.provider.DiscoverSetups()
	acct := config.Account{Name: "claude-default", Provider: "claude", Settings: setups[0].Settings}
	env, err := h.provider.Env(acct)
	if err != nil || !slices.Equal(env, []string{"CLAUDE_CONFIG_DIR=" + existing}) {
		t.Fatalf("adopted env %v err %v", env, err)
	}
	for _, item := range h.provider.DeleteItems(acct) {
		if item.Key == provider.DeleteKeyRemoveDir && (item.Default || !item.Locked) {
			t.Fatalf("adopted dir offered for removal: %+v", item)
		}
	}
	result, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{RemoveDir: true})
	if err != nil || len(result.Removed) != 0 {
		t.Fatalf("result %+v err %v", result, err)
	}
	if _, err := os.Stat(existing); err != nil {
		t.Fatalf("adopted dir removed: %v", err)
	}

	own := h.created(t, "own")
	if result, err := h.provider.Delete(context.Background(), own, provider.DeleteOptions{RemoveDir: true}); err != nil || len(result.Removed) != 1 {
		t.Fatalf("own dir: %+v %v", result, err)
	}
}

func TestClaudeNeverSurfacesSecrets(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	h.login(t, acct)
	// The real tool keeps tokens in its dir; the provider must never read them.
	if err := os.WriteFile(filepath.Join(h.root, "work", "auth.json"), []byte(`{"token":"`+fakeToken+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report := h.provider.CheckAuth(context.Background(), []config.Account{acct})
	local := h.provider.Local(acct)
	details := h.provider.Details(acct)
	_, setups, _ := h.provider.DiscoverSetups()
	all := fmt.Sprintf("%+v %+v %+v %+v", report, local, details, setups)
	for _, secret := range []string{fakeToken, fakeOrgID} {
		if strings.Contains(all, secret) {
			t.Fatalf("output contains %q: %s", secret, all)
		}
	}
}

func TestClaudeActivateCountsRunningProcesses(t *testing.T) {
	dir := t.TempDir()
	p := claudecode.New(claudecode.Options{
		ConfigRoot: filepath.Join(dir, "ai"),
		LookPath:   func(string) (string, error) { return fakeClaudeBin, nil },
		Getenv:     func(string) string { return "" },
		Processes: func(context.Context) ([]string, error) {
			return []string{"/usr/local/bin/claude", "zsh", "claude", "codex", "claude-helper"}, nil
		},
	})
	result, err := p.Activate(context.Background(), config.Account{Name: "work", Provider: "claude"})
	if err != nil || result.RunningProcesses != 2 || !strings.Contains(result.ApplyHint, "env work") {
		t.Fatalf("result %+v err %v", result, err)
	}

	failing := claudecode.New(claudecode.Options{
		ConfigRoot: filepath.Join(dir, "ai"),
		LookPath:   func(string) (string, error) { return fakeClaudeBin, nil },
		Getenv:     func(string) string { return "" },
		Processes:  func(context.Context) ([]string, error) { return nil, errors.New("no ps") },
	})
	if result, err := failing.Activate(context.Background(), config.Account{Name: "work"}); err != nil || result.RunningProcesses != 0 {
		t.Fatalf("ps failure must count 0: %+v %v", result, err)
	}
}
