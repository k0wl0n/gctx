package manager_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/internal/testutil"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

var cacheNow = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// cacheManager is dispatchManager with a temp status cache and a fixed clock.
func cacheManager(t *testing.T, now *time.Time, accounts []config.Account, providers ...provider.Provider) *manager.Manager {
	t.Helper()
	dir := t.TempDir()
	paths := manager.Paths{
		GctxDir:  filepath.Join(dir, "gctx"),
		CacheDir: filepath.Join(dir, "cache"),
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
		Now:               func() time.Time { return *now },
		Providers:         providers,
		CredentialTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStatusCacheHasNoSecrets(t *testing.T) {
	const plantedKey = "sk-or-v1-PLANTEDSECRET0123456789"
	fake := newFake(provider.AWS, "PROFILE")
	fake.envFn = func(config.Account) ([]string, error) { return []string{"OPENAI_API_KEY=" + plantedKey}, nil }
	fake.checkAuthFn = func(ctx context.Context, accts []config.Account) provider.AuthReport {
		_, _ = fake.Env(accts[0]) // the key is in reach of the check, as for a gateway
		return provider.AuthReport{Results: map[string]provider.AuthResult{"gw": {
			Credentialed: true,
			Identity:     "org-acme",
			Budget:       &provider.Budget{Spent: 3, Limit: 10, Currency: "USD"},
		}}}
	}
	now := cacheNow
	m := cacheManager(t, &now, []config.Account{{Name: "gw", ProjectID: "gw", Provider: "aws"}}, fake)
	if _, err := m.CheckCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(m.Paths().CacheDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), plantedKey) || !strings.Contains(string(data), "org-acme") {
		t.Fatalf("cache %s", data)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for name, child := range v {
				lower := strings.ToLower(name)
				for _, banned := range []string{"token", "key", "secret"} {
					if strings.Contains(lower, banned) && name != "gw" {
						t.Errorf("field %q in cache", name)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(doc)
}

func TestCheckCredentialsTimeoutBecomesUnverified(t *testing.T) {
	fake := newFake(provider.AWS, "PROFILE")
	result := provider.AuthResult{Credentialed: true, Identity: "arn:aws:sts::1:assumed-role/Dev/me"}
	fake.checkAuthFn = func(context.Context, []config.Account) provider.AuthReport {
		return provider.AuthReport{Results: map[string]provider.AuthResult{"a-aws": result}}
	}
	now := cacheNow
	m := cacheManager(t, &now, []config.Account{{Name: "a-aws", ProjectID: "dev", Provider: "aws"}}, fake)
	if _, err := m.CheckCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := status(t, m, "a-aws"); got.Status != manager.HealthReady || got.Identity != result.Identity || !got.Checked.Equal(cacheNow) {
		t.Fatalf("after good check: %+v", got)
	}

	now = cacheNow.Add(2 * time.Hour)
	result = provider.AuthResult{Err: context.DeadlineExceeded}
	if _, err := m.CheckCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := status(t, m, "a-aws")
	if got.Status != manager.HealthUnverified || got.HasFinding(manager.HealthAuthRequired) {
		t.Fatalf("after timeout: %s %+v", got.Status, got.Findings)
	}
	if got.LastGoodAge != 2*time.Hour || got.Identity != "arn:aws:sts::1:assumed-role/Dev/me" || got.Category != provider.CategoryCloud {
		t.Errorf("after timeout: %+v", got)
	}
}

func TestOverviewMergesCacheWithoutSubprocess(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("ok", "p1", testutil.SeedOptions{})
	env.SeedAccount("stale", "p2", testutil.SeedOptions{})
	m := newManager(t, env)
	checked := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	cache := map[string]any{"accounts": map[string]any{
		"ok":    map[string]any{"health": "READY", "identity": "ok@cached.example", "checked": checked},
		"stale": map[string]any{"health": "AUTH REQUIRED", "checked": checked},
	}}
	data, _ := json.Marshal(cache)
	env.WriteFile(filepath.Join(m.Paths().CacheDir, "status.json"), data, 0600)

	ok := status(t, m, "ok")
	if ok.Status != manager.HealthReady || ok.Identity != "ok@cached.example" || !ok.Checked.Equal(checked) {
		t.Errorf("ok: %+v", ok)
	}
	if stale := status(t, m, "stale"); stale.Status != manager.HealthAuthRequired {
		t.Errorf("stale: %s", stale.Status)
	}
	if calls := env.Invocations(); len(calls) != 0 {
		t.Errorf("Overview invoked %v", calls)
	}
}

func TestOverviewIgnoresCorruptCache(t *testing.T) {
	now := cacheNow
	m := cacheManager(t, &now, []config.Account{{Name: "a-aws", ProjectID: "dev", Provider: "aws"}}, newFake(provider.AWS, "PROFILE"))
	path := filepath.Join(m.Paths().CacheDir, "status.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("\x00garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := status(t, m, "a-aws"); got.Status != manager.HealthReady {
		t.Errorf("status %s", got.Status)
	}
}

func TestAbandonedProviderBecomesUnverified(t *testing.T) {
	fake := newFake(provider.AWS, "PROFILE")
	hang := false
	release := make(chan struct{})
	defer close(release)
	fake.checkAuthFn = func(context.Context, []config.Account) provider.AuthReport {
		if hang {
			<-release // hung: ignores ctx entirely
		}
		return provider.AuthReport{Results: map[string]provider.AuthResult{"a-aws": {Credentialed: true, Identity: "arn:aws:sts::1:assumed-role/Dev/me"}}}
	}
	now := cacheNow
	accounts := []config.Account{{Name: "a-aws", ProjectID: "dev", Provider: "aws"}, {Name: "b-aws", ProjectID: "prod", Provider: "aws"}}
	m := cacheManager(t, &now, accounts, fake)
	if _, err := m.CheckCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}

	now, hang = cacheNow.Add(2*time.Hour), true
	report, err := m.CheckCredentials(context.Background())
	if err != nil || len(report.Warnings) != 1 {
		t.Fatalf("%v %+v", err, report)
	}
	a := status(t, m, "a-aws")
	if a.Status != manager.HealthUnverified || a.LastGoodAge != 2*time.Hour || a.Identity != "arn:aws:sts::1:assumed-role/Dev/me" {
		t.Errorf("a-aws (cached last good): %+v", a)
	}
	// b-aws was AUTH REQUIRED before; a hung check must not keep or repeat that.
	b := status(t, m, "b-aws")
	if b.Status != manager.HealthUnverified || b.HasFinding(manager.HealthAuthRequired) {
		t.Errorf("b-aws: %s %+v", b.Status, b.Findings)
	}
}

func TestAbandonedIdentityKeyedProviderBecomesUnverified(t *testing.T) {
	fake := newFake(provider.GCP, "PROJECT")
	fake.localFn = func(config.Account) provider.LocalStatus {
		return provider.LocalStatus{Status: provider.HealthReady, Identity: "Me@Example.com"}
	}
	hang := false
	release := make(chan struct{})
	defer close(release)
	fake.checkAuthFn = func(context.Context, []config.Account) provider.AuthReport {
		if hang {
			<-release // hung: ignores ctx entirely
		}
		// Keyed by lower-cased identity, as GCP's ListAuth does.
		return provider.AuthReport{Results: map[string]provider.AuthResult{"me@example.com": {Credentialed: true, Identity: "Me@Example.com"}}}
	}
	now := cacheNow
	m := cacheManager(t, &now, []config.Account{{Name: "work", ConfigName: "work-config", ProjectID: "proj-one"}}, fake)
	if _, err := m.CheckCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := status(t, m, "work"); got.Status != manager.HealthReady {
		t.Fatalf("after good check: %s", got.Status)
	}

	now, hang = cacheNow.Add(2*time.Hour), true
	if _, err := m.CheckCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := status(t, m, "work")
	if got.Status != manager.HealthUnverified || got.LastGoodAge != 2*time.Hour || got.Identity != "Me@Example.com" {
		t.Errorf("after hung check: %s age=%s %+v", got.Status, got.LastGoodAge, got)
	}
}
