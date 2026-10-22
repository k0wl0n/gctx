package cmd_test

import (
	"testing"

	"github.com/k0wl0n/gctx/internal/testutil"
)

func TestActiveCommandListsSlots(t *testing.T) {
	env := testutil.NewEnv(t)
	env.WriteFile(env.ConfigPath(), []byte(`{
  "accounts": {
    "work": {"name": "work", "config_name": "work-config", "project_id": "p1", "adc_path": "", "created_at": "2026-04-01T10:00:00Z"},
    "prod": {"name": "prod", "config_name": "", "project_id": "dev", "adc_path": "", "created_at": "2026-04-01T10:00:00Z", "provider": "aws"}
  },
  "active_account": "work",
  "active_by_slot": {"aws": "prod"}
}`), 0644)
	assertStdout(t, runGctx(t, "active"), "gcp: work\naws: prod\n")
}

func TestActiveCommandGCPOnlyKeepsLegacyFormat(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SetActive("work")
	assertStdout(t, runGctx(t, "active"), "Active account: work\n")
}
