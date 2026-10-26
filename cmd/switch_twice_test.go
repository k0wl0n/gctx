package cmd_test

import (
	"testing"

	"github.com/k0wl0n/gctx/internal/testutil"
)

// TestSwitchTwiceKeepsLegacyOutput pins byte-identical GCP switch output when
// the gcp slot already held another account.
func TestSwitchTwiceKeepsLegacyOutput(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("home", "p2", testutil.SeedOptions{})

	assertCode(t, runGctx(t, "switch", "work"), 0)
	r := runGctx(t, "switch", "home")
	assertCode(t, r, 0)
	assertStdout(t, r, "Switched to account: home (p2)\n")
}
