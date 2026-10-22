package manager_test

import (
	"context"
	"slices"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func scopedShellManager(t *testing.T) *manager.Manager {
	t.Helper()
	gcpFake, awsFake := newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE")
	gcpFake.envFn = func(acct config.Account) ([]string, error) {
		return []string{"CLOUDSDK_ACTIVE_CONFIG_NAME=" + acct.Name + "-config"}, nil
	}
	awsFake.envFn = func(acct config.Account) ([]string, error) {
		return []string{"AWS_PROFILE=" + acct.Name}, nil
	}
	accounts := []config.Account{
		{Name: "g", ConfigName: "g-config", ProjectID: "proj"},
		{Name: "prod-aws", ProjectID: "prod", Provider: "aws"},
		{Name: "dev-aws", ProjectID: "dev", Provider: "aws"},
	}
	m, _ := dispatchManager(t, accounts, gcpFake, awsFake)
	ctx := context.Background()
	for _, name := range []string{"g", "prod-aws"} {
		if _, err := m.Switch(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func TestShellAllActiveMergesSlots(t *testing.T) {
	m := scopedShellManager(t)
	shell, err := m.ShellCommand("g", manager.ShellAllActive)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AWS_PROFILE=prod-aws", "CLOUDSDK_ACTIVE_CONFIG_NAME=g-config", "GCTX_ACCOUNT=g"} {
		if !slices.Contains(shell.Env, want) {
			t.Errorf("missing %s in shell env", want)
		}
	}
}

func TestShellOnlyHasSingleAccountEnv(t *testing.T) {
	m := scopedShellManager(t)
	shell, err := m.ShellCommand("g", manager.ShellOnly)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(shell.Env, "AWS_PROFILE=prod-aws") {
		t.Error("other slot leaked into the single-account shell")
	}
	if !slices.Contains(shell.Env, "CLOUDSDK_ACTIVE_CONFIG_NAME=g-config") {
		t.Error("selected account env missing")
	}
}

func TestSelectedAccountWinsInItsSlot(t *testing.T) {
	m := scopedShellManager(t)
	shell, err := m.ShellCommand("dev-aws", manager.ShellAllActive)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(shell.Env, "AWS_PROFILE=prod-aws") || !slices.Contains(shell.Env, "AWS_PROFILE=dev-aws") {
		t.Errorf("slot's active account overrode the selected one")
	}
	if !slices.Contains(shell.Env, "CLOUDSDK_ACTIVE_CONFIG_NAME=g-config") {
		t.Error("other active slot missing")
	}
}

type envBasedFake struct{ *fakeProvider }

func (envBasedFake) EnvBased() bool { return true }

func TestProviderInfoReportsEnvBased(t *testing.T) {
	m, _ := dispatchManager(t, twoProviderAccounts(), newFake(provider.GCP, "PROJECT"), envBasedFake{newFake(provider.AWS, "PROFILE")})
	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	got := map[provider.ID]bool{}
	for _, info := range overview.Providers {
		got[info.ID] = info.EnvBased
	}
	if got[provider.GCP] || !got[provider.AWS] {
		t.Errorf("EnvBased %v, want gcp false, aws true", got)
	}
}
