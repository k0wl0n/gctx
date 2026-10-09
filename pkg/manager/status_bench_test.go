package manager_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/internal/testutil"
	"github.com/k0wl0n/gctx/pkg/manager"
)

type benchEnvT struct {
	env *testutil.Env
	mgr *manager.Manager
	cleanup func()
}

func newBenchEnv(tb testing.TB, n int) benchEnvT {
	tb.Helper()
	env := testutil.NewEnv(tb)
	for i := range n {
		name := fmt.Sprintf("account%02d", i)
		proj := fmt.Sprintf("project-%02d", i)
		env.SeedAccount(name, proj, testutil.SeedOptions{})
	}
	m, err := manager.New()
	if err != nil {
		tb.Fatal(err)
	}
	return benchEnvT{env: env, mgr: m, cleanup: func() {}}
}

// BenchmarkOverview50 guards the < 100 ms single-run budget for 50 accounts.
func BenchmarkOverview50(b *testing.B) {
	be := newBenchEnv(b, 50)
	defer be.cleanup()

	b.ResetTimer()
	for range b.N {
		_, err := be.mgr.Overview()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestOverview50Under100ms asserts a single Overview() call for 50 accounts
// completes within 100 ms.
func TestOverview50Under100ms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}
	be := newBenchEnv(t, 50)
	defer be.cleanup()

	start := time.Now()
	_, err := be.mgr.Overview()
	dur := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if dur > 100*time.Millisecond {
		t.Errorf("Overview() took %v, want < 100ms", dur)
	}
}
