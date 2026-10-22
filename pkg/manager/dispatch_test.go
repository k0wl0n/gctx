package manager_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// fakeProvider is a scriptable Provider that records the operations it receives.
type fakeProvider struct {
	id          provider.ID
	label       string
	targetLabel string
	cli         provider.CLIStatus

	activateFn  func(ctx context.Context, acct config.Account) (provider.ActivateResult, error)
	localFn     func(acct config.Account) provider.LocalStatus
	checkAuthFn func(ctx context.Context, accts []config.Account) provider.AuthReport
	envFn       func(acct config.Account) ([]string, error)

	mu    sync.Mutex
	calls []string
}

var _ provider.Provider = (*fakeProvider)(nil)

func newFake(id provider.ID, targetLabel string) *fakeProvider {
	return &fakeProvider{
		id: id, label: strings.ToUpper(string(id)), targetLabel: targetLabel,
		cli: provider.CLIStatus{Bin: "/usr/bin/" + string(id), Found: true},
	}
}

func (f *fakeProvider) record(op string, acct config.Account) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, op+" "+acct.Name)
}

func (f *fakeProvider) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeProvider) ID() provider.ID                         { return f.id }
func (f *fakeProvider) Label() string                           { return f.label }
func (f *fakeProvider) TargetLabel() string                     { return f.targetLabel }
func (f *fakeProvider) CLI() provider.CLIStatus                 { return f.cli }
func (f *fakeProvider) ValidateTarget(string) error             { return nil }
func (f *fakeProvider) Details(config.Account) []provider.Field { return nil }
func (f *fakeProvider) Category() provider.Category             { return provider.CategoryCloud }
func (f *fakeProvider) Badge() provider.Badge                   { return provider.Badge{Text: f.label} }
func (f *fakeProvider) Slot() string                            { return string(f.id) }
func (f *fakeProvider) Columns() []provider.Column              { return nil }
func (f *fakeProvider) Form() []provider.FormField              { return nil }

func (f *fakeProvider) Create(_ context.Context, acct config.Account) (provider.CreateOutcome, error) {
	f.record("create", acct)
	return provider.CreateOutcome{}, nil
}

func (f *fakeProvider) Login(_ context.Context, acct config.Account, _ provider.LoginIO) (provider.LoginResult, error) {
	f.record("login", acct)
	return provider.LoginResult{Account: acct.Name}, nil
}

func (f *fakeProvider) Activate(ctx context.Context, acct config.Account) (provider.ActivateResult, error) {
	f.record("activate", acct)
	if f.activateFn != nil {
		return f.activateFn(ctx, acct)
	}
	return provider.ActivateResult{Account: acct.Name}, nil
}

func (f *fakeProvider) Local(acct config.Account) provider.LocalStatus {
	if f.localFn != nil {
		return f.localFn(acct)
	}
	return provider.LocalStatus{Status: provider.HealthReady}
}

func (f *fakeProvider) CheckAuth(ctx context.Context, accts []config.Account) provider.AuthReport {
	if f.checkAuthFn != nil {
		return f.checkAuthFn(ctx, accts)
	}
	return provider.AuthReport{}
}

func (f *fakeProvider) Env(acct config.Account) ([]string, error) {
	f.record("env", acct)
	if f.envFn != nil {
		return f.envFn(acct)
	}
	return []string{"FAKE_" + strings.ToUpper(string(f.id)) + "=" + acct.Name}, nil
}

func (f *fakeProvider) Delete(_ context.Context, acct config.Account, _ provider.DeleteOptions) (provider.DeleteResult, error) {
	f.record("delete", acct)
	return provider.DeleteResult{Account: acct.Name}, nil
}

// dispatchManager writes the given accounts to a fresh config.json and returns
// a Manager over the given providers.
func dispatchManager(t *testing.T, accounts []config.Account, providers ...provider.Provider) (*manager.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	paths := manager.Paths{
		GctxDir:    filepath.Join(dir, "gctx"),
		ADCDir:     filepath.Join(dir, "gctx", "adc"),
		GcloudDir:  filepath.Join(dir, "gcloud"),
		DefaultADC: filepath.Join(dir, "gcloud", "application_default_credentials.json"),
	}
	cfg, err := config.LoadFrom(paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	for i := range accounts {
		if err := cfg.AddAccount(&accounts[i]); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.NewWithOptions(manager.Options{
		Paths:             paths,
		Getenv:            func(string) string { return "" },
		Providers:         providers,
		CredentialTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, paths.ConfigPath()
}

func twoProviderAccounts() []config.Account {
	return []config.Account{
		{Name: "g", ConfigName: "g-config", ProjectID: "proj-one"},
		{Name: "a-aws", ProjectID: "dev", Provider: "aws"},
	}
}

func TestOperationsDispatchByProvider(t *testing.T) {
	gcpFake, awsFake := newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE")
	m, _ := dispatchManager(t, twoProviderAccounts(), gcpFake, awsFake)

	if _, err := m.Switch(context.Background(), "a-aws"); err != nil {
		t.Fatal(err)
	}
	if got := awsFake.Calls(); !slices.Equal(got, []string{"activate a-aws"}) {
		t.Errorf("aws calls %v", got)
	}
	if got := gcpFake.Calls(); len(got) != 0 {
		t.Errorf("gcp called on aws switch: %v", got)
	}

	if _, err := m.Delete(context.Background(), "g", manager.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := gcpFake.Calls(); !slices.Equal(got, []string{"delete g"}) {
		t.Errorf("gcp calls %v", got)
	}
	if got := awsFake.Calls(); len(got) != 1 {
		t.Errorf("aws called on gcp delete: %v", got)
	}
}

func TestEnvDispatches(t *testing.T) {
	gcpFake, awsFake := newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE")
	m, _ := dispatchManager(t, twoProviderAccounts(), gcpFake, awsFake)

	env, err := m.Env("a-aws")
	if err != nil || !slices.Equal(env, []string{"FAKE_AWS=a-aws"}) {
		t.Fatalf("aws env %v %v", env, err)
	}
	env, err = m.Env("g")
	if err != nil || !slices.Equal(env, []string{"FAKE_GCP=g"}) {
		t.Fatalf("gcp env %v %v", env, err)
	}
	if _, err := m.Env("missing"); !errors.Is(err, manager.ErrAccountNotFound) {
		t.Errorf("missing account: %v", err)
	}
}

func TestMissingCLIAffectsOnlyThatProvider(t *testing.T) {
	gcpFake, awsFake := newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE")
	awsFake.cli = provider.CLIStatus{Bin: "", Found: false}
	accounts := append(twoProviderAccounts(), config.Account{Name: "b-aws", ProjectID: "prod", Provider: "aws"})
	m, _ := dispatchManager(t, accounts, gcpFake, awsFake)

	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Accounts) != 3 {
		t.Fatalf("accounts %+v", overview.Accounts)
	}
	for _, name := range []string{"a-aws", "b-aws"} {
		account, _ := overview.Account(name)
		if account.Status != provider.HealthCLIMissing || account.Provider != provider.AWS || account.TargetLabel != "PROFILE" {
			t.Errorf("%s: %+v", name, account)
		}
	}
	gcpAccount, _ := overview.Account("g")
	if gcpAccount.Status != manager.HealthReady || gcpAccount.Provider != provider.GCP || gcpAccount.TargetLabel != "PROJECT" {
		t.Errorf("g: %+v", gcpAccount)
	}

	want := []manager.ProviderInfo{
		{ID: provider.GCP, Label: "GCP", CLI: gcpFake.cli, Count: 1},
		{ID: provider.AWS, Label: "AWS", CLI: awsFake.cli, Count: 2},
	}
	if !slices.Equal(overview.Providers, want) {
		t.Errorf("providers %+v, want %+v", overview.Providers, want)
	}
}

func TestCheckCredentialsTimeoutIsPerProvider(t *testing.T) {
	gcpFake, awsFake := newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE")
	gcpFake.checkAuthFn = func(context.Context, []config.Account) provider.AuthReport {
		return provider.AuthReport{Results: map[string]provider.AuthResult{"g": {Credentialed: true}}}
	}
	release := make(chan struct{})
	defer close(release)
	awsFake.checkAuthFn = func(ctx context.Context, _ []config.Account) provider.AuthReport {
		<-release // hung: ignores ctx entirely
		return provider.AuthReport{}
	}
	m, _ := dispatchManager(t, twoProviderAccounts(), gcpFake, awsFake)

	start := time.Now()
	report, err := m.CheckCredentials(context.Background())
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("check took %s", elapsed)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(report.Accounts, "g") {
		t.Errorf("gcp results missing: %+v", report)
	}
	if len(report.Warnings) != 1 || report.Warnings[0].Code != manager.WarnCredentialCheck || !strings.Contains(report.Warnings[0].Message, "AWS") {
		t.Errorf("warnings %+v", report.Warnings)
	}
	if got := status(t, m, "g").Status; got != manager.HealthReady {
		t.Errorf("g after check: %s", got)
	}
	if got := status(t, m, "a-aws").Status; got != manager.HealthUnverified {
		t.Errorf("a-aws must be UNVERIFIED after a timed-out check: %s", got)
	}
}

func TestCheckCredentialsMarksUncredentialedByName(t *testing.T) {
	awsFake := newFake(provider.AWS, "PROFILE")
	awsFake.checkAuthFn = func(context.Context, []config.Account) provider.AuthReport {
		return provider.AuthReport{Results: map[string]provider.AuthResult{"a-aws": {Credentialed: true}}}
	}
	accounts := append(twoProviderAccounts(), config.Account{Name: "b-aws", ProjectID: "prod", Provider: "aws"})
	m, _ := dispatchManager(t, accounts, newFake(provider.GCP, "PROJECT"), awsFake)

	if _, err := m.CheckCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := status(t, m, "a-aws").Status; got != manager.HealthReady {
		t.Errorf("a-aws: %s", got)
	}
	if got := status(t, m, "b-aws").Status; got != manager.HealthAuthRequired {
		t.Errorf("b-aws: %s", got)
	}
}

func TestSwitchEnvProviderReturnsApplyHint(t *testing.T) {
	awsFake := newFake(provider.AWS, "PROFILE")
	awsFake.activateFn = func(_ context.Context, acct config.Account) (provider.ActivateResult, error) {
		return provider.ActivateResult{Account: acct.Name, ApplyHint: "eval \"$(gctx env a-aws)\""}, nil
	}
	m, configPath := dispatchManager(t, twoProviderAccounts(), newFake(provider.GCP, "PROJECT"), awsFake)

	result, err := m.Switch(context.Background(), "a-aws")
	if err != nil {
		t.Fatal(err)
	}
	if result.ApplyHint == "" || len(result.Warnings) != 0 {
		t.Errorf("result %+v", result)
	}
	cfg, err := config.LoadFrom(configPath)
	if err != nil || cfg.ActiveBySlot["aws"] != "a-aws" || cfg.ActiveAccount != "" {
		t.Errorf("active slots %v account %q %v", cfg.ActiveBySlot, cfg.ActiveAccount, err)
	}
}

func TestUnknownProviderListedAsError(t *testing.T) {
	accounts := append(twoProviderAccounts(), config.Account{Name: "future", ProjectID: "x", Provider: "oracle"})
	m, _ := dispatchManager(t, accounts, newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE"))

	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	future, ok := overview.Account("future")
	if !ok || future.Status != manager.HealthError || len(future.Findings) != 1 || !strings.Contains(future.Findings[0].Message, "oracle") {
		t.Errorf("future: %+v", future)
	}
	if g, _ := overview.Account("g"); g.Status != manager.HealthReady {
		t.Errorf("g: %+v", g)
	}
	_, err = m.Switch(context.Background(), "future")
	if !errors.Is(err, manager.ErrUnknownProvider) || !strings.Contains(err.Error(), "oracle") {
		t.Errorf("switch: %v", err)
	}
	if _, err := m.CheckCredentials(context.Background()); err != nil {
		t.Errorf("check: %v", err)
	}
}
