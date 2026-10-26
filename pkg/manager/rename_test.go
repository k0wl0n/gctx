package manager_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/internal/testutil"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

const subA = "8c1f0a3e-0000-4000-8000-00000000e2a9"

func TestRenameGCPMovesStoredADCAndSwitchRestoresIt(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("a", "proj-one", testutil.SeedOptions{})
	m := newManager(t, env)
	ctx := context.Background()

	if err := m.UpdateAccount(ctx, "a", manager.UpdateRequest{NewName: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.StoredADCPath("a")); !os.IsNotExist(err) {
		t.Error("stored ADC still under the old name")
	}
	if got := env.ReadConfig().Accounts["b"].ADCPath; got != env.StoredADCPath("b") {
		t.Errorf("ADCPath %q, want %q", got, env.StoredADCPath("b"))
	}
	result, err := m.Switch(ctx, "b")
	if err != nil || hasWarning(result.Warnings, manager.WarnADCMissing) {
		t.Fatalf("switch b: %+v err %v", result, err)
	}
	if data, _ := os.ReadFile(env.DefaultADCPath()); !bytes.Equal(data, testutil.ADCJSON("a")) {
		t.Error("switch did not restore the renamed account's ADC")
	}
}

func TestCreateRefusesArtefactsOwnedByARenamedAccount(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("a", "proj-one", testutil.SeedOptions{})
	m := newManager(t, env)
	ctx := context.Background()
	if err := m.UpdateAccount(ctx, "a", manager.UpdateRequest{NewName: "b"}); err != nil {
		t.Fatal(err)
	}
	_, err := m.Create(ctx, "a", "proj-two")
	if err == nil || !strings.Contains(err.Error(), `"b"`) {
		t.Fatalf("create a after rename: %v, want a refusal naming b", err)
	}
	if _, ok := env.ReadConfig().Accounts["a"]; ok {
		t.Error("account a was created")
	}
}

func TestGCPDeleteKeepsArtefactsAnotherAccountRecords(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("b", "proj-one", testutil.SeedOptions{})
	env.SetGcloudActiveConfig("other-config")
	cfg := env.ReadConfig()
	legacy := *cfg.Accounts["b"]
	legacy.Name = "a"
	cfg.Accounts["a"] = &legacy // a legacy rename left a and b sharing b's files
	env.WriteConfig(cfg)
	env.ResetInvocations()

	m := newManager(t, env)
	if _, err := m.Delete(context.Background(), "a", manager.DeleteOptions{GcloudConfig: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.GcloudConfigPath("b-config")); err != nil {
		t.Error("gcloud configuration still used by b was deleted")
	}
	if _, err := os.Stat(env.StoredADCPath("b")); err != nil {
		t.Error("stored ADC still used by b was deleted")
	}
	if lines := env.InvocationLines(); len(lines) != 0 {
		t.Errorf("gcloud calls %v", lines)
	}
}

func azureFixture(t *testing.T) (discoveryFixture, string) {
	t.Helper()
	t.Setenv("PATH", "")
	fx := newDiscoveryFixture(t, func(name string) (string, error) { return "/fake/" + name, nil })
	return fx, filepath.Join(fx.paths.GctxDir, "azure")
}

func TestRenameAzureMovesDirAndFreesTheOldName(t *testing.T) {
	fx, root := azureFixture(t)
	ctx := context.Background()
	if _, err := fx.manager.CreateAccount(ctx, manager.CreateRequest{Provider: provider.Azure, Name: "a", Target: subA}); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(root, "a", "msal_token_cache.json"), "FAKE-TOKENS")
	if err := fx.manager.UpdateAccount(ctx, "a", manager.UpdateRequest{NewName: "b"}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b", "msal_token_cache.json")); err != nil || string(data) != "FAKE-TOKENS" {
		t.Errorf("tokens not moved: %q %v", data, err)
	}
	cfg, _ := config.LoadFrom(fx.paths.ConfigPath())
	if got := cfg.Accounts["b"].Settings["dir"]; got != filepath.Join(root, "b") {
		t.Errorf("dir setting %q", got)
	}
	if _, err := fx.manager.CreateAccount(ctx, manager.CreateRequest{Provider: provider.Azure, Name: "a", Target: subA}); err != nil {
		t.Fatalf("create a after rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b", "msal_token_cache.json")); err != nil {
		t.Error("b's tokens disturbed by the new a")
	}
}

func TestAzureCreateImportAndDeleteRespectAnotherAccountsDir(t *testing.T) {
	fx, root := azureFixture(t)
	ctx := context.Background()
	cfg, _ := config.LoadFrom(fx.paths.ConfigPath())
	legacy := &config.Account{Name: "b", ProjectID: subA, Provider: "azure", Settings: map[string]string{"dir": filepath.Join(root, "a")}}
	if err := cfg.AddAccount(legacy); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(root, "a", "msal_token_cache.json"), "FAKE-TOKENS")

	_, err := fx.manager.CreateAccount(ctx, manager.CreateRequest{Provider: provider.Azure, Name: "a", Target: subA})
	if err == nil || !strings.Contains(err.Error(), `"b"`) {
		t.Errorf("create: %v, want a refusal naming b", err)
	}
	result, err := fx.manager.ImportAccounts([]manager.DiscoveredAccount{{
		Provider: provider.Azure, Name: "a", Target: subA, Settings: map[string]string{"dir": filepath.Join(root, "a")},
	}})
	if err != nil || len(result.Imported) != 0 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, `"b"`) {
		t.Errorf("import %+v err %v, want a skip naming b", result, err)
	}

	cfg, _ = config.LoadFrom(fx.paths.ConfigPath())
	twin := &config.Account{Name: "c", ProjectID: subA, Provider: "azure", Settings: map[string]string{"dir": filepath.Join(root, "a")}}
	nested := &config.Account{Name: "d", ProjectID: subA, Provider: "azure", Settings: map[string]string{"dir": filepath.Join(root, "a", "sub")}}
	if cfg.AddAccount(twin) != nil || cfg.AddAccount(nested) != nil {
		t.Fatal("seed")
	}
	writeFixtureFile(t, filepath.Join(root, "a", "sub", "x"), "x")
	for _, name := range []string{"c", "d"} {
		if _, err := fx.manager.Delete(ctx, name, manager.DeleteOptions{RemoveDir: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "a", "sub", "x")); err != nil {
		t.Error("delete removed a directory that is not the account's own")
	}
}

func TestImportRefusesGCPConfigOwnedByAnotherAccount(t *testing.T) {
	t.Setenv("PATH", "")
	fx := newDiscoveryFixture(t, nil)
	cfg, _ := config.LoadFrom(fx.paths.ConfigPath())
	if err := cfg.AddAccount(&config.Account{Name: "b", ProjectID: "default-project", ConfigName: "default"}); err != nil {
		t.Fatal(err)
	}
	result, err := fx.manager.ImportAccounts([]manager.DiscoveredAccount{{Provider: provider.GCP, Name: "other", Target: "default-project", ConfigName: "default"}})
	if err != nil || len(result.Imported) != 0 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, `"b"`) {
		t.Errorf("import %+v err %v", result, err)
	}
}

// fixedNameFake cannot rename: its resources follow the account name.
type fixedNameFake struct{ *fakeProvider }

func (fixedNameFake) CanRename() bool { return false }

func TestRenameRefusedWithoutRenameSupport(t *testing.T) {
	m, path := dispatchManager(t, []config.Account{{Name: "x", ProjectID: "p", Provider: "aws"}}, fixedNameFake{newFake(provider.AWS, "PROFILE")})
	if err := m.UpdateAccount(context.Background(), "x", manager.UpdateRequest{NewName: "y"}); err == nil {
		t.Fatal("rename accepted")
	}
	if cfg, _ := config.LoadFrom(path); cfg.Accounts["x"] == nil {
		t.Error("x renamed despite the refusal")
	}
}
