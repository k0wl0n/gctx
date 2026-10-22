package manager_test

import (
	"context"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// gatewayFake is a fake provider that reports the shared openai-api slot.
type gatewayFake struct{ *fakeProvider }

func (gatewayFake) Slot() string { return "openai-api" }

func slotsManager(t *testing.T, providers ...provider.Provider) (*manager.Manager, string) {
	accounts := []config.Account{
		{Name: "g", ConfigName: "g-config", ProjectID: "proj"},
		{Name: "a-aws", ProjectID: "dev", Provider: "aws"},
		{Name: "gw-one", ProjectID: "x", Provider: "gw1"},
		{Name: "gw-two", ProjectID: "y", Provider: "gw2"},
	}
	return dispatchManager(t, accounts, providers...)
}

func activeSlots(t *testing.T, m *manager.Manager) map[string]string {
	t.Helper()
	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	return overview.ActiveBySlot
}

func TestSwitchSetsOnlyItsSlot(t *testing.T) {
	m, _ := slotsManager(t, newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE"))
	ctx := context.Background()
	if _, err := m.Switch(ctx, "g"); err != nil {
		t.Fatal(err)
	}
	result, err := m.Switch(ctx, "a-aws")
	if err != nil {
		t.Fatal(err)
	}
	if result.Slot != "aws" || result.Replaced != "" {
		t.Errorf("result slot=%q replaced=%q", result.Slot, result.Replaced)
	}
	slots := activeSlots(t, m)
	if slots["gcp"] != "g" || slots["aws"] != "a-aws" {
		t.Errorf("slots %v", slots)
	}
}

func TestGatewaysShareOpenAISlot(t *testing.T) {
	m, _ := slotsManager(t, newFake(provider.GCP, "PROJECT"),
		gatewayFake{newFake("gw1", "URL")}, gatewayFake{newFake("gw2", "URL")})
	ctx := context.Background()
	if _, err := m.Switch(ctx, "gw-one"); err != nil {
		t.Fatal(err)
	}
	result, err := m.Switch(ctx, "gw-two")
	if err != nil {
		t.Fatal(err)
	}
	if result.Slot != "openai-api" || result.Replaced != "gw-one" {
		t.Errorf("result slot=%q replaced=%q", result.Slot, result.Replaced)
	}
	again, _ := m.Switch(ctx, "gw-two")
	if again.Replaced != "" {
		t.Errorf("same account reported as replaced: %q", again.Replaced)
	}
}

func TestDeleteActiveClearsSlot(t *testing.T) {
	m, _ := slotsManager(t, newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE"))
	ctx := context.Background()
	m.Switch(ctx, "a-aws")
	if _, err := m.Delete(ctx, "a-aws", manager.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := activeSlots(t, m); got["aws"] != "" {
		t.Errorf("aws slot still %q", got["aws"])
	}
}

func TestOverviewActiveBySlot(t *testing.T) {
	m, _ := slotsManager(t, newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE"))
	ctx := context.Background()
	m.Switch(ctx, "g")
	m.Switch(ctx, "a-aws")
	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.ActiveAccount != "g" {
		t.Errorf("ActiveAccount %q", overview.ActiveAccount)
	}
	if active, ok := overview.Active(); !ok || active.Name != "g" {
		t.Errorf("Active() %v %v", active.Name, ok)
	}
	for _, name := range []string{"g", "a-aws"} {
		status, _ := overview.Account(name)
		if !status.Active {
			t.Errorf("%s not active in its slot", name)
		}
	}
}
