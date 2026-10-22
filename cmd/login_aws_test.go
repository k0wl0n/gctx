package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/internal/testutil"
)

func TestLoginAWSProfileShowsMissingKeysWarning(t *testing.T) {
	env := testutil.NewEnv(t)
	binDir := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, "aws"), "github.com/k0wl0n/gctx/internal/testutil/fakeaws").CombinedOutput(); err != nil {
		t.Fatalf("building fakeaws: %v\n%s", err, out)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	env.WriteFile(filepath.Join(env.Home, ".aws", "config"),
		[]byte("[profile work]\nsso_start_url = https://example.awsapps.com/start\nsso_region = eu-west-1\n"), 0o644)
	addAccountOfProvider(t, env, "work-aws", "aws", "work")

	r := runGctx(t, "login", "work-aws")
	assertCode(t, r, 0)
	for _, want := range []string{"aws configure set profile.work.sso_account_id ACCOUNT_ID", "aws configure set profile.work.sso_role_name ROLE_NAME"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr %q lacks %q", r.stderr, want)
		}
	}
	if strings.Contains(r.stdout, "ADC") || strings.Contains(r.stdout, "Saved to") {
		t.Errorf("stdout %q has ADC lines", r.stdout)
	}
}
