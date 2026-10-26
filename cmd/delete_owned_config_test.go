package cmd_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/internal/testutil"
)

// markConfigImported records the account's gcloud configuration as the user's own.
func markConfigImported(t *testing.T, env *testutil.Env, name string) {
	t.Helper()
	data, err := os.ReadFile(env.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	account := doc["accounts"].(map[string]any)[name].(map[string]any)
	account["settings"] = map[string]any{"config_origin": "imported"}
	data, _ = json.Marshal(doc)
	env.WriteFile(env.ConfigPath(), data, 0o600)
}

func TestDeleteGcloudConfigRefusesUserOwnedConfigWithoutForce(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	env.SetGcloudActiveConfig("other-config")
	markConfigImported(t, env, "sandbox")

	r := runGctx(t, "delete", "sandbox", "--gcloud-config")
	if r.code == 0 || !strings.Contains(r.stderr, "--force") {
		t.Fatalf("code %d stderr %q, want a refusal naming --force", r.code, r.stderr)
	}
	if _, err := os.Stat(env.GcloudConfigPath("sandbox-config")); err != nil {
		t.Error("user-owned gcloud config was touched")
	}
	if _, ok := env.ReadConfig().Accounts["sandbox"]; !ok {
		t.Error("account removed despite the refusal")
	}

	r = runGctx(t, "delete", "sandbox", "--gcloud-config", "--force")
	assertCode(t, r, 0)
	assertCalls(t, env, "config configurations delete sandbox-config --quiet")
}
