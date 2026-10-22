package manager_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestCheckProviderCredentialsChecksOnlyThatProvider(t *testing.T) {
	gcpFake, awsFake := newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE")
	gcpChecked := false
	gcpFake.checkAuthFn = func(context.Context, []config.Account) provider.AuthReport {
		gcpChecked = true
		return provider.AuthReport{}
	}
	awsFake.checkAuthFn = func(context.Context, []config.Account) provider.AuthReport {
		return provider.AuthReport{Results: map[string]provider.AuthResult{"a-aws": {Credentialed: true, Identity: "arn:aws:sts::1:assumed-role/dev"}}}
	}
	m, _ := dispatchManager(t, twoProviderAccounts(), gcpFake, awsFake)

	report, err := m.CheckProviderCredentials(context.Background(), provider.AWS)
	if err != nil {
		t.Fatal(err)
	}
	if gcpChecked {
		t.Error("gcp checked by an aws-only check")
	}
	if !slices.Equal(report.Accounts, []string{"a-aws"}) || len(report.Warnings) != 0 {
		t.Errorf("report %+v", report)
	}
	if got := status(t, m, "a-aws"); got.Status != manager.HealthReady || got.Identity == "" {
		t.Errorf("a-aws after check: %+v", got)
	}
}

func TestCheckProviderCredentialsTimeoutIsUnverified(t *testing.T) {
	awsFake := newFake(provider.AWS, "PROFILE")
	release := make(chan struct{})
	defer close(release)
	awsFake.checkAuthFn = func(context.Context, []config.Account) provider.AuthReport {
		<-release
		return provider.AuthReport{}
	}
	m, _ := dispatchManager(t, twoProviderAccounts(), newFake(provider.GCP, "PROJECT"), awsFake)

	start := time.Now()
	report, err := m.CheckProviderCredentials(context.Background(), provider.AWS)
	if err != nil || time.Since(start) > time.Second {
		t.Fatalf("err %v after %s", err, time.Since(start))
	}
	if len(report.Warnings) != 1 || report.Warnings[0].Code != manager.WarnCredentialCheck {
		t.Errorf("warnings %+v", report.Warnings)
	}
	if got := status(t, m, "a-aws").Status; got != manager.HealthUnverified {
		t.Errorf("a-aws after timeout: %s", got)
	}
}

func TestCheckProviderCredentialsUnknownProvider(t *testing.T) {
	m, _ := dispatchManager(t, twoProviderAccounts(), newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE"))
	if _, err := m.CheckProviderCredentials(context.Background(), "nope"); err == nil {
		t.Error("unknown provider accepted")
	}
}

func TestProvidersCarryMetadata(t *testing.T) {
	awsFake := newFake(provider.AWS, "PROFILE")
	accounts := append(twoProviderAccounts(), config.Account{Name: "b-aws", ProjectID: "ops", Provider: "aws", Settings: map[string]string{"region": "eu-west-1"}})
	m, _ := dispatchManager(t, accounts, newFake(provider.GCP, "PROJECT"), awsFake)

	infos := m.Providers()
	if len(infos) != 2 {
		t.Fatalf("providers %+v", infos)
	}
	aws := infos[1]
	if aws.ID != provider.AWS || aws.Category != provider.CategoryCloud || aws.Badge.Text != "AWS" || aws.Slot != "aws" ||
		aws.TargetLabel != "PROFILE" || aws.Count != 2 || !aws.CLI.Found || aws.CheckTimeout != 50*time.Millisecond {
		t.Errorf("aws info %+v", aws)
	}
	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(overview.Providers, infos, func(a, b manager.ProviderInfo) bool { return a.ID == b.ID && a.Count == b.Count }) {
		t.Errorf("overview providers %+v", overview.Providers)
	}
	b, _ := overview.Account("b-aws")
	if b.Settings["region"] != "eu-west-1" {
		t.Errorf("settings %+v", b.Settings)
	}
}
