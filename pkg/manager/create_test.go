package manager_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestCreateAccountDispatchesToProvider(t *testing.T) {
	gcpFake := newFake(provider.GCP, "PROJECT")
	awsFake := newFake(provider.AWS, "PROFILE")
	m, configPath := dispatchManager(t, nil, gcpFake, awsFake)

	result, err := m.CreateAccount(context.Background(), manager.CreateRequest{
		Provider: provider.AWS, Name: "prod", Target: "prod-profile",
		Fields: map[string]string{"region": "eu-west-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(awsFake.Calls(), []string{"create prod"}) || len(gcpFake.Calls()) != 0 {
		t.Fatalf("aws calls %v, gcp calls %v", awsFake.Calls(), gcpFake.Calls())
	}
	if result.Account != "prod" || result.ProjectID != "prod-profile" {
		t.Errorf("result %+v", result)
	}
	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	stored := cfg.Accounts["prod"]
	if stored == nil || stored.Provider != "aws" || stored.ProjectID != "prod-profile" || stored.Settings["region"] != "eu-west-1" {
		t.Fatalf("stored %+v", stored)
	}
}

func TestCreateAccountDefaultsToGCP(t *testing.T) {
	gcpFake := newFake(provider.GCP, "PROJECT")
	m, _ := dispatchManager(t, nil, gcpFake)
	if _, err := m.CreateAccount(context.Background(), manager.CreateRequest{Name: "work", Target: "p1"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(gcpFake.Calls(), []string{"create work"}) {
		t.Fatalf("calls %v", gcpFake.Calls())
	}
}

func TestCreateAccountUnknownProviderListsValid(t *testing.T) {
	m, _ := dispatchManager(t, nil, newFake(provider.GCP, "PROJECT"))
	_, err := m.CreateAccount(context.Background(), manager.CreateRequest{Provider: "nope", Name: "x", Target: "t"})
	if !errors.Is(err, manager.ErrUnknownProvider) || !strings.Contains(err.Error(), "valid: gcp") {
		t.Fatalf("err %v", err)
	}
}
