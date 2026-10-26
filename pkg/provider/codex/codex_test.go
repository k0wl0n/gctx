package codex_test

import (
	"bufio"
	"context"
	"encoding/json"
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
	"github.com/k0wl0n/gctx/pkg/provider/codex"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

var (
	_ provider.Provider        = (*codex.Provider)(nil)
	_ provider.SetupDiscoverer = (*codex.Provider)(nil)
)

const fakeToken = "FAKE-CODEX-SECRET-TOKEN"

var fakeCodexBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gctx-fakecodex-")
	if err != nil {
		panic(err)
	}
	fakeCodexBin = filepath.Join(dir, "codex")
	if out, err := exec.Command("go", "build", "-o", fakeCodexBin, "github.com/k0wl0n/gctx/internal/testutil/fakecodex").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakecodex: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type call struct {
	Args      []string `json:"args"`
	CodexHome string   `json:"codex_home"`
}

func readCalls(t *testing.T, path string) []call {
	t.Helper()
	f, err := os.Open(path)
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

func newProvider(t *testing.T) (p *codex.Provider, root, home, logPath string) {
	t.Helper()
	dir := t.TempDir()
	root, home, logPath = filepath.Join(dir, "gctx", "ai"), filepath.Join(dir, "home"), filepath.Join(dir, "calls.log")
	p = codex.New(codex.Options{
		ConfigRoot: root,
		Home:       home,
		LookPath:   func(string) (string, error) { return fakeCodexBin, nil },
		Getenv:     func(string) string { return "" },
		Runner:     execrunner.New(fakeCodexBin).WithEnv([]string{"FAKE_CODEX_LOG=" + logPath}),
	})
	return p, root, home, logPath
}

func TestCodexEnvAndLogin(t *testing.T) {
	p, root, _, logPath := newProvider(t)
	if p.ID() != provider.Codex || p.Slot() != "codex" || p.Category() != provider.CategoryAITool || p.Badge().Text != "OAI" {
		t.Fatalf("metadata %s %s %s %+v", p.ID(), p.Slot(), p.Category(), p.Badge())
	}
	// An account whose dir is gone (never created, or removed): login must
	// recreate it, because codex refuses a CODEX_HOME that does not exist.
	acct := config.Account{Name: "work", Provider: "codex", Settings: map[string]string{}}
	dir := filepath.Join(root, "work")
	if _, err := p.Login(context.Background(), acct, provider.LoginIO{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir %v err %v", info, err)
	}
	calls := readCalls(t, logPath)
	if len(calls) != 1 || strings.Join(calls[0].Args, " ") != "login" || calls[0].CodexHome != dir {
		t.Fatalf("calls %+v", calls)
	}
	env, err := p.Env(acct)
	if err != nil || !slices.Equal(env, []string{"CODEX_HOME=" + dir}) {
		t.Fatalf("env %v err %v", env, err)
	}

	other := config.Account{Name: "other", Provider: "codex", Settings: map[string]string{}}
	outcome, err := p.Create(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(other.Settings, outcome.Settings)
	report := p.CheckAuth(context.Background(), []config.Account{acct, other})
	if res := report.Results["work"]; !res.Credentialed || res.Err != nil {
		t.Fatalf("work %+v", res)
	}
	if res := report.Results["other"]; res.Credentialed || res.Err != nil {
		t.Fatalf("other %+v", res)
	}
	missing := config.Account{Name: "gone", Provider: "codex"}
	if res := p.CheckAuth(context.Background(), []config.Account{missing}).Results["gone"]; res.Credentialed || res.Err != nil {
		t.Fatalf("missing dir must be not logged in: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "gone")); !os.IsNotExist(err) {
		t.Fatal("CheckAuth created a directory")
	}

	all := fmt.Sprintf("%+v %+v %+v", report, p.Details(acct), p.Local(acct))
	if strings.Contains(all, fakeToken) {
		t.Fatalf("token surfaced: %s", all)
	}
}

func TestCodexDiscoverAdoptsExistingDir(t *testing.T) {
	p, _, home, _ := newProvider(t)
	existing := filepath.Join(home, ".codex")
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	location, setups, err := p.DiscoverSetups()
	if err != nil || location != existing || len(setups) != 1 || setups[0].Name != "codex-default" || setups[0].Settings[provider.SettingDir] != existing {
		t.Fatalf("location %q setups %+v err %v", location, setups, err)
	}
	acct := config.Account{Name: "codex-default", Provider: "codex", Settings: setups[0].Settings}
	if _, err := p.Delete(context.Background(), acct, provider.DeleteOptions{RemoveDir: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(existing); err != nil {
		t.Fatalf("adopted dir removed: %v", err)
	}
}

func TestCodexDefaultDirAccountLeavesHomeUnset(t *testing.T) {
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "leaked"))
	dir := t.TempDir()
	root, home, logPath := filepath.Join(dir, "gctx", "ai"), filepath.Join(dir, "home"), filepath.Join(dir, "calls.log")
	defaultDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := codex.New(codex.Options{
		ConfigRoot: root,
		Home:       home,
		LookPath:   func(string) (string, error) { return fakeCodexBin, nil },
		Getenv:     func(string) string { return "" },
		Runner:     execrunner.New(fakeCodexBin).WithEnv([]string{"FAKE_CODEX_LOG=" + logPath, "FAKE_CODEX_DEFAULT_DIR=" + defaultDir}),
	})
	adopted := config.Account{Name: "codex-default", Provider: "codex", Settings: map[string]string{
		provider.SettingDir:          defaultDir,
		provider.SettingConfigOrigin: provider.ConfigOriginImported,
	}}
	if env, err := p.Env(adopted); err != nil || !slices.Equal(env, []string{"CODEX_HOME"}) {
		t.Fatalf("env %v err %v: want the variable unset", env, err)
	}
	if _, err := p.Login(context.Background(), adopted, provider.LoginIO{}); err != nil {
		t.Fatal(err)
	}
	if res := p.CheckAuth(context.Background(), []config.Account{adopted}).Results["codex-default"]; !res.Credentialed || res.Err != nil {
		t.Fatalf("check %+v", res)
	}
	for _, c := range readCalls(t, logPath) {
		if c.CodexHome != "" {
			t.Fatalf("%v ran with CODEX_HOME=%q", c.Args, c.CodexHome)
		}
	}
}
