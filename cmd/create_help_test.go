package cmd_test

import (
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/internal/testutil"
)

// TestCreateHelpListsRegisteredProvidersOnly keeps unsupported providers out of the help.
func TestCreateHelpListsRegisteredProvidersOnly(t *testing.T) {
	testutil.NewEnv(t)
	r := runGctx(t, "create", "--help")
	assertCode(t, r, 0)
	if strings.Contains(r.stdout, "tencent") || !strings.Contains(r.stdout, "provider: gcp, aws, azure, claude or codex") {
		t.Errorf("create help:\n%s", r.stdout)
	}
}
