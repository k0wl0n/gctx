package manager_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

var (
	fakeAIOnce sync.Once
	fakeAIDir  string
	fakeAIErr  error
)

// fakeAIBins builds fakeclaude and fakecodex once and returns their directory.
func fakeAIBins(t *testing.T) string {
	t.Helper()
	fakeAIOnce.Do(func() {
		if fakeAIDir, fakeAIErr = os.MkdirTemp("", "gctx-fakeai-"); fakeAIErr != nil {
			return
		}
		for name, pkg := range map[string]string{"claude": "fakeclaude", "codex": "fakecodex"} {
			out, err := exec.Command("go", "build", "-o", filepath.Join(fakeAIDir, name), "github.com/k0wl0n/gctx/internal/testutil/"+pkg).CombinedOutput()
			if err != nil {
				fakeAIErr = fmt.Errorf("building %s: %v\n%s", pkg, err, out)
				return
			}
		}
	})
	if fakeAIErr != nil {
		t.Fatal(fakeAIErr)
	}
	return fakeAIDir
}

type aiFixture struct {
	home    string
	paths   manager.Paths
	env     map[string]string
	manager *manager.Manager
}

// newAIFixture is a manager over the default provider set: claude and codex
// resolve to the fakes, every other CLI to a path that is never run.
func newAIFixture(t *testing.T, accounts []config.Account, processes func(context.Context) ([]string, error)) aiFixture {
	t.Helper()
	bins := fakeAIBins(t)
	t.Setenv("FAKE_CLAUDE_LOG", filepath.Join(t.TempDir(), "claude.log"))
	home := t.TempDir()
	env := map[string]string{}
	paths := manager.ResolvePaths(func(k string) string { return env[k] }, "linux", home)
	cfg, err := config.LoadFrom(paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, acct := range accounts {
		if err := cfg.AddAccount(&acct); err != nil {
			t.Fatal(err)
		}
	}
	if len(accounts) > 0 {
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.NewWithOptions(manager.Options{
		Paths:  paths,
		Home:   home,
		Getenv: func(k string) string { return env[k] },
		Gcloud: &gcloud.Client{Bin: "/nonexistent/gcloud"},
		LookPath: func(name string) (string, error) {
			if name == "claude" || name == "codex" {
				return filepath.Join(bins, name), nil
			}
			return "/nonexistent/" + name, nil
		},
		Processes: processes,
		Now:       func() time.Time { return time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return aiFixture{home: home, paths: paths, env: env, manager: m}
}

func gcpAccount() config.Account {
	return config.Account{Name: "gcp-prod", ProjectID: "my-project", ConfigName: "gcp-prod-config"}
}

func TestDefaultSetRegistersAITools(t *testing.T) {
	fx := newAIFixture(t, nil, nil)
	var aiTools []provider.ID
	for _, info := range fx.manager.Providers() {
		if info.Category == provider.CategoryAITool {
			aiTools = append(aiTools, info.ID)
		}
	}
	if !slices.Equal(aiTools, []provider.ID{provider.ClaudeCode, provider.Codex}) {
		t.Fatalf("ai tools %v", aiTools)
	}
}

func TestClaudeViaVertexEnvMergesLinkedAccount(t *testing.T) {
	fx := newAIFixture(t, []config.Account{gcpAccount(), {
		Name: "claude-vertex", Provider: "claude",
		Settings: map[string]string{"via": "gcp-prod", "backend": "vertex", "region": "europe-west1"},
	}}, nil)
	linkedEnv, err := fx.manager.Env("gcp-prod")
	if err != nil {
		t.Fatal(err)
	}
	env, err := fx.manager.Env("claude-vertex")
	if err != nil {
		t.Fatal(err)
	}
	want := append(slices.Clone(linkedEnv),
		"CLAUDE_CONFIG_DIR="+filepath.Join(fx.paths.GctxDir, "ai", "claude-vertex"),
		"CLAUDE_CODE_USE_VERTEX=1", "ANTHROPIC_VERTEX_PROJECT_ID=my-project", "CLOUD_ML_REGION=europe-west1")
	if !slices.Equal(env, want) {
		t.Fatalf("env\n got %v\nwant %v", env, want)
	}
}

func TestClaudeViaInheritsLinkedHealth(t *testing.T) {
	fx := newAIFixture(t, []config.Account{gcpAccount(), {
		Name: "claude-vertex", Provider: "claude",
		Settings: map[string]string{"via": "gcp-prod", "backend": "vertex"},
	}}, nil)
	overview, err := fx.manager.Overview()
	if err != nil {
		t.Fatal(err)
	}
	linked, _ := overview.Account("gcp-prod")
	tool, _ := overview.Account("claude-vertex")
	if linked.Status == manager.HealthReady {
		t.Fatalf("fixture: linked account should need action, got %s", linked.Status)
	}
	if tool.Status != linked.Status || tool.Linked != "gcp-prod" {
		t.Fatalf("tool status %s linked %q; linked account %s", tool.Status, tool.Linked, linked.Status)
	}

	// A credential check never asks claude about a linked account.
	_, _ = fx.manager.CheckProviderCredentials(context.Background(), provider.ClaudeCode)
	overview, _ = fx.manager.Overview()
	if tool, _ = overview.Account("claude-vertex"); tool.Status != linked.Status {
		t.Fatalf("after check: %s", tool.Status)
	}
}

func TestClaudeViaRejectsUnsupportedCloud(t *testing.T) {
	azureAccount := config.Account{Name: "corp-azure", Provider: "azure", ProjectID: "12345678-1234-1234-1234-123456789abc"}
	fx := newAIFixture(t, []config.Account{azureAccount, gcpAccount(), {
		Name: "hand-edited", Provider: "claude", Settings: map[string]string{"via": "corp-azure"},
	}}, nil)

	_, err := fx.manager.CreateAccount(context.Background(), manager.CreateRequest{
		Provider: provider.ClaudeCode, Name: "claude-azure",
		Fields: map[string]string{"auth": "via cloud account", "via": "corp-azure"},
	})
	if err == nil || !strings.Contains(err.Error(), "Azure") {
		t.Fatalf("create on azure: %v", err)
	}
	if _, err := fx.manager.CreateAccount(context.Background(), manager.CreateRequest{
		Provider: provider.ClaudeCode, Name: "claude-ghost",
		Fields: map[string]string{"auth": "via cloud account", "via": "nobody"},
	}); err == nil {
		t.Fatal("create via a missing account accepted")
	}
	if _, err := fx.manager.Env("hand-edited"); err == nil || !strings.Contains(err.Error(), "Azure") {
		t.Fatalf("env on azure: %v", err)
	}
	overview, _ := fx.manager.Overview()
	if acct, _ := overview.Account("hand-edited"); acct.Status != manager.HealthError {
		t.Fatalf("status %s", acct.Status)
	}

	if _, err := fx.manager.CreateAccount(context.Background(), manager.CreateRequest{
		Provider: provider.ClaudeCode, Name: "claude-vertex",
		Fields: map[string]string{"auth": "via cloud account", "via": "gcp-prod", "backend": "vertex", "region": "us-east5"},
	}); err != nil {
		t.Fatalf("create via gcp: %v", err)
	}
	if _, err := fx.manager.Delete(context.Background(), "gcp-prod", manager.DeleteOptions{}); !errors.As(err, new(manager.ErrAccountLinked)) {
		t.Fatalf("deleting the linked account: %v", err)
	}
}

func TestRunningProcessWarning(t *testing.T) {
	processes := func(context.Context) ([]string, error) {
		return []string{"/opt/homebrew/bin/claude", "claude", "/bin/zsh", "codex"}, nil
	}
	fx := newAIFixture(t, []config.Account{{Name: "claude-work", Provider: "claude"}}, processes)
	result, err := fx.manager.Switch(context.Background(), "claude-work")
	if err != nil {
		t.Fatal(err)
	}
	if result.RunningProcesses != 2 || result.Slot != "claude" || result.ApplyHint == "" {
		t.Fatalf("result %+v", result)
	}
}

func TestDiscoverAdoptsExistingAIToolDirs(t *testing.T) {
	fx := newAIFixture(t, []config.Account{{Name: "claude-default", ProjectID: "taken", ConfigName: "taken"}}, nil)
	claudeDir, codexDir := filepath.Join(fx.home, ".claude"), filepath.Join(fx.home, ".codex")
	for dir, file := range map[string]string{claudeDir: ".credentials.json", codexDir: "auth.json"} {
		// Unreadable secrets: discovery must not need to open them.
		writeFixtureFile(t, filepath.Join(dir, file), `{"token":"FAKE-AI-SECRET-TOKEN"}`)
		if err := os.Chmod(filepath.Join(dir, file), 0); err != nil {
			t.Fatal(err)
		}
	}

	result, err := fx.manager.Discover()
	if err != nil {
		t.Fatal(err)
	}
	claude, codex := itemsOf(result, provider.ClaudeCode), itemsOf(result, provider.Codex)
	if len(claude) != 1 || len(codex) != 1 {
		t.Fatalf("claude %v codex %v", claude, codex)
	}
	claudeItem, ok := claude["claude-default-2"]
	if !ok || claudeItem.Settings["dir"] != claudeDir {
		t.Fatalf("claude item must be unique-ified and keep the dir: %+v", claude)
	}
	if item := codex["codex-default"]; item.Settings["dir"] != codexDir {
		t.Fatalf("codex item %+v", item)
	}
	var sourceNouns []string
	for _, source := range result.Sources {
		if source.Provider == provider.ClaudeCode {
			sourceNouns = append(sourceNouns, fmt.Sprint(source.Count, " ", source.Noun))
		}
	}
	if !slices.Equal(sourceNouns, []string{"1 existing setup in ~/.claude"}) {
		t.Fatalf("sources %v", sourceNouns)
	}

	imported, err := fx.manager.ImportAccounts([]manager.DiscoveredAccount{claudeItem, codex["codex-default"]})
	if err != nil || len(imported.Imported) != 2 {
		t.Fatalf("import %+v err %v", imported, err)
	}
	env, err := fx.manager.Env("claude-default-2")
	if err != nil || !slices.Equal(env, []string{"CLAUDE_CONFIG_DIR=" + claudeDir}) {
		t.Fatalf("env %v err %v", env, err)
	}
	if again, _ := fx.manager.Discover(); len(itemsOf(again, provider.ClaudeCode))+len(itemsOf(again, provider.Codex)) != 0 {
		t.Fatalf("adopted dirs offered again: %+v", again.Items)
	}

	report, _ := fx.manager.CheckProviderCredentials(context.Background(), provider.ClaudeCode)
	overview, _ := fx.manager.Overview()
	config, _ := os.ReadFile(fx.paths.ConfigPath())
	if all := fmt.Sprintf("%+v %+v %s", report, overview, config); strings.Contains(all, "FAKE-AI-SECRET-TOKEN") {
		t.Fatalf("secret surfaced: %s", all)
	}

	for _, name := range []string{"claude-default-2", "codex-default"} {
		if _, err := fx.manager.Delete(context.Background(), name, manager.DeleteOptions{RemoveDir: true, Logout: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{claudeDir, codexDir} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("deleting the account removed %s: %v", dir, err)
		}
	}
}

func TestClaudeViaEditRejectsUnsupportedCloud(t *testing.T) {
	azureAccount := config.Account{Name: "corp-azure", Provider: "azure", ProjectID: "12345678-1234-1234-1234-123456789abc"}
	fx := newAIFixture(t, []config.Account{azureAccount, gcpAccount(), {
		Name: "claude-vertex", Provider: "claude", Settings: map[string]string{"via": "gcp-prod", "backend": "vertex"},
	}}, nil)
	err := fx.manager.UpdateAccount(context.Background(), "claude-vertex", manager.UpdateRequest{Settings: map[string]string{"via": "corp-azure"}})
	if err == nil {
		t.Fatal("relinking to an Azure account accepted")
	}
	if env, err := fx.manager.Env("claude-vertex"); err != nil || !slices.Contains(env, "CLAUDE_CODE_USE_VERTEX=1") {
		t.Fatalf("link changed after a refused edit: %v %v", env, err)
	}
}
