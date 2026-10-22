package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/provider"
)

var testNow = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func TestUnverifiedOnTimeoutKeepsLastGood(t *testing.T) {
	last := &cachedResult{Health: HealthReady, Identity: "me@example.com", Checked: testNow.Add(-2 * time.Hour)}
	got := deriveHealth(provider.AuthResult{Err: context.DeadlineExceeded, Checked: testNow}, last, testNow)
	if got.Health != HealthUnverified || got.LastGoodAge != 2*time.Hour || got.Identity != "me@example.com" || !got.Checked.Equal(last.Checked) {
		t.Errorf("outcome %+v", got)
	}
}

func TestTimeoutNeverBecomesAuthRequired(t *testing.T) {
	errs := []error{context.DeadlineExceeded, errors.New("dial tcp: no route to host")}
	lasts := []*cachedResult{nil, {Health: HealthAuthRequired, Checked: testNow.Add(-time.Minute)}}
	for _, err := range errs {
		for _, last := range lasts {
			got := deriveHealth(provider.AuthResult{Err: err}, last, testNow)
			if got.Health != HealthUnverified {
				t.Errorf("err %v last %+v: %s", err, last, got.Health)
			}
			if last == nil && got.LastGoodAge != 0 {
				t.Errorf("age without a cached result: %s", got.LastGoodAge)
			}
		}
	}
}

func TestExpiredFromExpiresAt(t *testing.T) {
	for _, tt := range []struct {
		expires time.Time
		want    Health
	}{
		{testNow.Add(-time.Second), HealthExpired},
		{testNow.Add(time.Hour), HealthReady},
		{time.Time{}, HealthReady},
	} {
		got := deriveHealth(provider.AuthResult{Credentialed: true, ExpiresAt: tt.expires}, nil, testNow)
		if got.Health != tt.want || !got.ExpiresAt.Equal(tt.expires) {
			t.Errorf("expires %s: %+v, want %s", tt.expires, got, tt.want)
		}
	}
}

func TestBudgetLowBelowTenPercent(t *testing.T) {
	for _, tt := range []struct {
		name   string
		budget *provider.Budget
		want   Health
	}{
		{"below ten percent", &provider.Budget{Spent: 91, Limit: 100, Currency: "USD"}, HealthBudgetLow},
		{"exactly ten percent", &provider.Budget{Spent: 90, Limit: 100}, HealthReady},
		{"unlimited", &provider.Budget{Spent: 1e6}, HealthReady},
		{"no budget", nil, HealthReady},
	} {
		got := deriveHealth(provider.AuthResult{Credentialed: true, Budget: tt.budget}, nil, testNow)
		if got.Health != tt.want || got.Budget != tt.budget {
			t.Errorf("%s: %+v, want %s", tt.name, got, tt.want)
		}
	}
}

func TestDeriveHealthTable(t *testing.T) {
	past, future := testNow.Add(-time.Hour), testNow.Add(time.Hour)
	low := &provider.Budget{Spent: 95, Limit: 100}
	for _, tt := range []struct {
		name string
		res  provider.AuthResult
		want Health
	}{
		{"credentialed", provider.AuthResult{Credentialed: true}, HealthReady},
		{"not credentialed", provider.AuthResult{}, HealthAuthRequired},
		{"not credentialed beats expiry", provider.AuthResult{ExpiresAt: past}, HealthAuthRequired},
		{"expired", provider.AuthResult{Credentialed: true, ExpiresAt: past}, HealthExpired},
		{"expired beats budget", provider.AuthResult{Credentialed: true, ExpiresAt: past, Budget: low}, HealthExpired},
		{"budget low", provider.AuthResult{Credentialed: true, ExpiresAt: future, Budget: low}, HealthBudgetLow},
		{"error beats everything", provider.AuthResult{Credentialed: true, ExpiresAt: past, Budget: low, Err: errors.New("x")}, HealthUnverified},
	} {
		if got := deriveHealth(tt.res, nil, testNow); got.Health != tt.want {
			t.Errorf("%s: %s, want %s", tt.name, got.Health, tt.want)
		}
	}
}

func TestStatusCacheMergeKeepsLastGoodOnError(t *testing.T) {
	cache := statusCache{}
	cache.Merge(map[string]provider.AuthResult{"a": {Credentialed: true, Identity: "id-a"}}, testNow)
	cache.Merge(map[string]provider.AuthResult{"a": {Err: context.DeadlineExceeded}}, testNow.Add(time.Hour))
	got := cache.Accounts["a"]
	if got.Health != HealthReady || got.Identity != "id-a" || !got.Checked.Equal(testNow) {
		t.Errorf("entry %+v", got)
	}
}

func TestStatusCacheFileMode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	dir := filepath.Join(t.TempDir(), "cache")
	path := filepath.Join(dir, "status.json")
	cache := statusCache{}
	cache.Merge(map[string]provider.AuthResult{"a": {Credentialed: true}}, testNow)
	if err := cache.Save(path); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{path: 0600, dir: 0700} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v %v, want %v", p, info.Mode().Perm(), err, want)
		}
	}
	var loaded statusCache
	loaded.Load(path)
	if loaded.Accounts["a"].Health != HealthReady {
		t.Errorf("round trip %+v", loaded)
	}
}

func TestCorruptCacheIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	var cache statusCache
	cache.Load(path)
	if len(cache.Accounts) != 0 {
		t.Errorf("corrupt cache loaded %+v", cache)
	}
	cache.Load(filepath.Join(t.TempDir(), "missing.json"))
	if len(cache.Accounts) != 0 {
		t.Errorf("missing cache loaded %+v", cache)
	}
}
