package manager_test

import (
	"context"
	"errors"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// editableFake has one editable and one identity form field.
type editableFake struct{ *fakeProvider }

func (editableFake) Form() []provider.FormField {
	return []provider.FormField{
		{Key: provider.FormTargetKey, Label: "Target"},
		{Key: "region", Label: "Region", Editable: true},
	}
}

func updateManager(t *testing.T) (*manager.Manager, string) {
	t.Helper()
	accounts := []config.Account{
		{Name: "prod", ProjectID: "p1", Provider: "aws", Settings: map[string]string{"region": "eu-west-1", "auth": "sso"}},
		{Name: "dev", ProjectID: "p2", Provider: "aws"},
	}
	return dispatchManager(t, accounts, editableFake{newFake(provider.AWS, "PROFILE")})
}

func TestUpdateAccountRenameRejectsDuplicate(t *testing.T) {
	m, path := updateManager(t)
	err := m.UpdateAccount(context.Background(), "prod", manager.UpdateRequest{NewName: "dev"})
	if !errors.Is(err, manager.ErrAccountExists) {
		t.Fatalf("err = %v, want ErrAccountExists", err)
	}
	cfg, loadErr := config.LoadFrom(path)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if cfg.Accounts["prod"] == nil {
		t.Error("prod must be untouched after a rejected rename")
	}
}

func TestUpdateAccountRenameKeepsActiveSlot(t *testing.T) {
	m, path := updateManager(t)
	ctx := context.Background()
	if _, err := m.Switch(ctx, "prod"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateAccount(ctx, "prod", manager.UpdateRequest{NewName: "prod-eu"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveBySlot["aws"] != "prod-eu" {
		t.Errorf("active slot = %q, want prod-eu", cfg.ActiveBySlot["aws"])
	}
	if cfg.Accounts["prod"] != nil || cfg.Accounts["prod-eu"] == nil || cfg.Accounts["prod-eu"].Name != "prod-eu" {
		t.Fatalf("accounts after rename: %+v", cfg.Accounts)
	}
	if cfg.Accounts["prod-eu"].Settings["auth"] != "sso" {
		t.Error("rename must keep the other settings")
	}
}

func TestUpdateAccountMergesEditableSettingsOnly(t *testing.T) {
	m, path := updateManager(t)
	ctx := context.Background()
	if err := m.UpdateAccount(ctx, "prod", manager.UpdateRequest{Settings: map[string]string{"region": "us-east-1"}}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.LoadFrom(path)
	if got := cfg.Accounts["prod"].Settings["region"]; got != "us-east-1" {
		t.Errorf("region = %q", got)
	}
	if err := m.UpdateAccount(ctx, "prod", manager.UpdateRequest{Settings: map[string]string{"auth": "profile"}}); err == nil {
		t.Error("changing a read-only identity setting must be refused")
	}
	if err := m.UpdateAccount(ctx, "prod", manager.UpdateRequest{NewName: "Bad Name"}); !errors.Is(err, manager.ErrInvalidName) {
		t.Errorf("invalid new name err = %v", err)
	}
	if err := m.UpdateAccount(ctx, "ghost", manager.UpdateRequest{NewName: "x"}); !errors.Is(err, manager.ErrAccountNotFound) {
		t.Errorf("unknown account err = %v", err)
	}
}
