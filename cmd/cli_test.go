package cmd_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/internal/testutil"
)

var update = flag.Bool("update", false, "rewrite golden files")

var gctxBin string

func TestMain(m *testing.M) {
	flag.Parse()
	dir, err := os.MkdirTemp("", "gctx-cli-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	gctxBin = filepath.Join(dir, "gctx")
	if runtime.GOOS == "windows" {
		gctxBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", gctxBin, "github.com/k0wl0n/gctx")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building gctx: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
}

// runGctx runs the built binary with stdin attached to the null device, so it
// never sees a terminal.
func runGctx(t *testing.T, args ...string) result {
	t.Helper()
	cmd := exec.Command(gctxBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// normalize replaces the temp root with <ROOT> and uses forward slashes so
// goldens are identical on every OS.
func normalize(env *testutil.Env, s string) string {
	s = strings.ReplaceAll(s, env.Root, "<ROOT>")
	if runtime.GOOS == "windows" {
		s = strings.ReplaceAll(s, `\`, "/")
	}
	return s
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func assertCode(t *testing.T, r result, want int) {
	t.Helper()
	if r.code != want {
		t.Fatalf("exit code %d, want %d\nstdout: %s\nstderr: %s", r.code, want, r.stdout, r.stderr)
	}
}

func assertStdout(t *testing.T, r result, want string) {
	t.Helper()
	if r.stdout != want {
		t.Errorf("stdout mismatch\n--- got ---\n%q\n--- want ---\n%q", r.stdout, want)
	}
}

func assertCalls(t *testing.T, env *testutil.Env, want ...string) {
	t.Helper()
	got := env.InvocationLines()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("gcloud calls\n--- got ---\n%s\n--- want ---\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRootHelp(t *testing.T) {
	testutil.NewEnv(t)
	bare := runGctx(t)
	assertCode(t, bare, 0)
	if bare.stderr != "" {
		t.Errorf("unexpected stderr %q", bare.stderr)
	}
	assertGolden(t, "root-help", bare.stdout)

	help := runGctx(t, "--help")
	assertCode(t, help, 0)
	if help.stdout != bare.stdout {
		t.Error("gctx --help differs from bare gctx output")
	}
}

func TestUnknownCommand(t *testing.T) {
	testutil.NewEnv(t)
	r := runGctx(t, "foo")
	assertCode(t, r, 1)
	if strings.Count(r.stderr, `Error: unknown command "foo" for "gctx"`) != 2 {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestList(t *testing.T) {
	env := testutil.NewEnv(t)
	r := runGctx(t, "list")
	assertCode(t, r, 0)
	assertStdout(t, r, "No accounts configured\n")

	env.SeedAccount("work", "p1", testutil.SeedOptions{Email: "w@x.com"})
	env.SeedAccount("alpha", "p2", testutil.SeedOptions{})
	env.SeedAccount("mid", "p3", testutil.SeedOptions{})
	cfg := env.ReadConfig()
	cfg.Accounts["alpha"].Email = ""
	cfg.ActiveAccount = "work"
	env.WriteConfig(cfg)

	r = runGctx(t, "list")
	assertCode(t, r, 0)
	assertStdout(t, r, "\nConfigured Accounts:\n===================\n  alpha (p2)\n  mid (p3) [mid@example.com]\n  work (p1) [w@x.com] ← active\n")
}

func TestActive(t *testing.T) {
	env := testutil.NewEnv(t)
	assertStdout(t, runGctx(t, "active"), "Active account: unknown\n")

	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SetActive("work")
	assertStdout(t, runGctx(t, "active"), "Active account: work\n")

	t.Setenv("GCTX_ACCOUNT", "personal")
	assertStdout(t, runGctx(t, "active"), "Active account: personal\n")
}

func TestActiveWithArgumentSwitches(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	r := runGctx(t, "active", "work")
	assertCode(t, r, 0)
	assertStdout(t, r, "Switched to account: work (p1)\n")
}

func TestSwitch(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})

	r := runGctx(t, "switch", "work")
	assertCode(t, r, 0)
	assertStdout(t, r, "Switched to account: work (p1)\n")
	assertCalls(t, env, "config configurations activate work-config", "config set project p1")

	if env.ReadConfig().ActiveAccount != "work" {
		t.Error("active account not set")
	}
	adc, _ := os.ReadFile(env.DefaultADCPath())
	if !bytes.Equal(adc, testutil.ADCJSON("work")) {
		t.Error("default ADC not restored")
	}
	if env.GcloudActiveConfig() != "work-config" {
		t.Error("gcloud config not activated")
	}
}

func TestSwitchWithoutADC(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{NoADC: true})

	r := runGctx(t, "switch", "sandbox")
	assertCode(t, r, 0)
	assertStdout(t, r, "\nWarning: No saved ADC credentials for account 'sandbox'.\n"+
		"Run 'gctx login sandbox' to authenticate and save credentials.\n\n"+
		"Switched to account: sandbox (p2)\n")
	if env.ReadConfig().ActiveAccount != "sandbox" {
		t.Error("active account not set")
	}
}

func TestSwitchReauthRequired(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	t.Setenv("FAKE_GCLOUD_FAIL", "config-set:1:ERROR: (gcloud.config.set) Reauthentication required.")

	r := runGctx(t, "switch", "sandbox")
	assertCode(t, r, 0)
	assertStdout(t, r, "\nWarning: Failed to set project ID because re-authentication is required.\n"+
		"Please run: gctx login sandbox\n\n"+
		"Switched to account: sandbox (p2)\n")
}

func TestSwitchActivateFails(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	env.SetActive("work")
	t.Setenv("FAKE_GCLOUD_FAIL", "configurations-activate:1")

	r := runGctx(t, "switch", "sandbox")
	assertCode(t, r, 1)
	if !strings.Contains(r.stderr, "Error: ") {
		t.Errorf("stderr %q", r.stderr)
	}
	if env.ReadConfig().ActiveAccount != "work" {
		t.Error("active account must be unchanged")
	}
}

func TestSwitchUnknownAccount(t *testing.T) {
	testutil.NewEnv(t)
	r := runGctx(t, "switch", "nope")
	assertCode(t, r, 1)
	if strings.Count(r.stderr, "Error: account 'nope' not found") != 2 {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestSave(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{NoADC: true})
	env.SetGcloudActiveConfig("work-config")

	r := runGctx(t, "save", "work")
	assertCode(t, r, 1)
	if !strings.Contains(normalize(env, r.stderr), "no ADC found at <ROOT>/") {
		t.Errorf("stderr %q", r.stderr)
	}

	env.WriteFile(env.DefaultADCPath(), testutil.ADCJSON("current"), 0600)
	r = runGctx(t, "save", "work")
	assertCode(t, r, 0)
	assertStdout(t, r, fmt.Sprintf("ADC credentials saved for: work\nLocation: %s\n", env.StoredADCPath("work")))

	stored, _ := os.ReadFile(env.StoredADCPath("work"))
	if !bytes.Equal(stored, testutil.ADCJSON("current")) {
		t.Error("stored ADC differs from default ADC")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(env.StoredADCPath("work"))
		if info.Mode().Perm() != 0600 {
			t.Errorf("stored ADC mode %v", info.Mode().Perm())
		}
	}
	cfg := env.ReadConfig()
	if cfg.Accounts["work"].ADCPath != env.StoredADCPath("work") || cfg.Accounts["work"].Email != "work@example.com" {
		t.Errorf("account not updated: %+v", cfg.Accounts["work"])
	}
}

func TestSaveUnknownAccount(t *testing.T) {
	testutil.NewEnv(t)
	assertCode(t, runGctx(t, "save", "nope"), 1)
}

func TestDelete(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SetGcloudActiveConfig("work-config")

	r := runGctx(t, "delete", "sandbox")
	assertCode(t, r, 0)
	assertStdout(t, r, "Deleted account: sandbox\n")
	if _, ok := env.ReadConfig().Accounts["sandbox"]; ok {
		t.Error("account still configured")
	}
	if _, err := os.Stat(env.StoredADCPath("sandbox")); !os.IsNotExist(err) {
		t.Error("stored ADC not removed")
	}
	if _, err := os.Stat(env.GcloudConfigPath("sandbox-config")); err != nil {
		t.Error("gcloud config must be kept without --gcloud-config")
	}
	for _, line := range env.InvocationLines() {
		if strings.Contains(line, "configurations delete") {
			t.Errorf("unexpected gcloud call %q", line)
		}
	}
}

func TestDeleteWithGcloudConfig(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	env.SetGcloudActiveConfig("other-config")

	r := runGctx(t, "delete", "sandbox", "--gcloud-config")
	assertCode(t, r, 0)
	assertStdout(t, r, "Deleted account: sandbox\n")
	assertCalls(t, env, "config configurations delete sandbox-config --quiet")
	if _, err := os.Stat(env.GcloudConfigPath("sandbox-config")); !os.IsNotExist(err) {
		t.Error("gcloud config not deleted")
	}
}

func TestRun(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("personal", "p9", testutil.SeedOptions{})
	env.SetActive("work")

	r := runGctx(t, "run", "personal", "config", "get-value", "project")
	assertCode(t, r, 0)
	assertStdout(t, r, "p9\n")
	calls := env.Invocations()
	if len(calls) != 1 || calls[0].ActiveConfig != "personal-config" || calls[0].ADCEnv != env.StoredADCPath("personal") {
		t.Errorf("calls %+v", calls)
	}
	if env.ReadConfig().ActiveAccount != "work" {
		t.Error("run must not change the active account")
	}
}

func TestRunPassesFlags(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	r := runGctx(t, "run", "work", "storage", "buckets", "list", "--limit=1")
	assertCode(t, r, 0)
	assertCalls(t, env, "storage buckets list --limit=1")
}

func TestRunFailure(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	t.Setenv("FAKE_GCLOUD_FAIL", "other:3")
	r := runGctx(t, "run", "work", "storage", "ls")
	assertCode(t, r, 1)
	if !strings.Contains(r.stderr, "Error: exit status 3") {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestShell(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("personal", "p9", testutil.SeedOptions{})
	t.Setenv("SHELL", env.FakeShell())

	r := runGctx(t, "shell", "personal")
	assertCode(t, r, 0)
	assertStdout(t, r, "Starting shell for account 'personal'...\n"+
		"Type 'exit' to return to the previous context.\n"+
		"GCTX_ACCOUNT=personal\n"+
		"CLOUDSDK_ACTIVE_CONFIG_NAME=personal-config\n"+
		"GOOGLE_APPLICATION_CREDENTIALS="+env.StoredADCPath("personal")+"\n")
}

func TestInfo(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{Email: "w@x.com"})
	env.SetActive("work")
	modified := time.Date(2026, 5, 2, 3, 4, 5, 0, time.Local)
	if err := os.Chtimes(env.StoredADCPath("work"), modified, modified); err != nil {
		t.Fatal(err)
	}

	r := runGctx(t, "info", "work")
	assertCode(t, r, 0)
	assertGolden(t, "info", normalize(env, r.stdout))

	r = runGctx(t, "info", "nope")
	assertCode(t, r, 1)
	if !strings.Contains(r.stderr, "Error: account 'nope' not found") {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestCompletion(t *testing.T) {
	testutil.NewEnv(t)
	for _, shell := range []string{"bash", "zsh", "fish"} {
		r := runGctx(t, "completion", shell)
		assertCode(t, r, 0)
		if len(r.stdout) < 100 {
			t.Errorf("%s completion too short", shell)
		}
	}
	r := runGctx(t, "completion", "foo")
	assertCode(t, r, 1)
	if !strings.Contains(r.stderr, "unsupported shell: foo") {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestVersion(t *testing.T) {
	testutil.NewEnv(t)
	r := runGctx(t, "version")
	assertCode(t, r, 0)
	lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
	prefixes := []string{"gctx version ", "commit: ", "built at: ", "go version: ", "os/arch: "}
	if len(lines) != len(prefixes) {
		t.Fatalf("version output %q", r.stdout)
	}
	for i, prefix := range prefixes {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Errorf("line %d %q lacks prefix %q", i, lines[i], prefix)
		}
	}
}

func TestCreate(t *testing.T) {
	env := testutil.NewEnv(t)

	r := runGctx(t, "create", "demo", "demo-project")
	assertCode(t, r, 0)
	assertGolden(t, "create", normalize(env, r.stdout))

	cfg := env.ReadConfig()
	demo := cfg.Accounts["demo"]
	if demo == nil || demo.ConfigName != "demo-config" || demo.ProjectID != "demo-project" ||
		demo.ADCPath != env.StoredADCPath("demo") || demo.Email != "demo@example.com" {
		t.Fatalf("account %+v", demo)
	}
	if env.GcloudProperty("demo-config", "project") != "demo-project" {
		t.Error("project not set")
	}
}

func TestCreateDuplicate(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("demo", "p1", testutil.SeedOptions{})
	r := runGctx(t, "create", "demo", "demo-project")
	assertCode(t, r, 1)
	if !strings.Contains(r.stderr, "Error: account 'demo' already exists") {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestCreateAuthLoginFails(t *testing.T) {
	env := testutil.NewEnv(t)
	t.Setenv("FAKE_GCLOUD_FAIL", "auth-login:1")
	r := runGctx(t, "create", "demo", "demo-project")
	assertCode(t, r, 1)
	if !strings.HasSuffix(r.stdout, "Running authentication...\n") {
		t.Errorf("stdout %q", r.stdout)
	}
	if !strings.Contains(r.stderr, "Error: auth login failed: exit status 1") {
		t.Errorf("stderr %q", r.stderr)
	}
	if env.ReadConfig().Accounts["demo"] == nil {
		t.Error("account record must remain after failed login")
	}
}

func TestCreateADCLoginFails(t *testing.T) {
	env := testutil.NewEnv(t)
	t.Setenv("FAKE_GCLOUD_FAIL", "auth-adc-login:1")
	r := runGctx(t, "create", "demo", "demo-project")
	assertCode(t, r, 1)
	assertGolden(t, "create-adc-fails", normalize(env, r.stdout))
	if !strings.Contains(r.stderr, "Error: ADC auth failed: exit status 1") {
		t.Errorf("stderr %q", r.stderr)
	}
}

func TestLogin(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{NoADC: true})

	r := runGctx(t, "login", "work")
	assertCode(t, r, 0)
	assertGolden(t, "login", normalize(env, r.stdout))

	calls := env.InvocationLines()
	sort.Strings(calls)
	want := []string{"auth application-default login", "auth login", "config configurations activate work-config", "config get-value account", "config set project p1"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls %v", calls)
	}
	if env.ReadConfig().Accounts["work"].ADCPath != env.StoredADCPath("work") {
		t.Error("adc_path not recorded")
	}
}

func TestLoginUnknownAccount(t *testing.T) {
	testutil.NewEnv(t)
	assertCode(t, runGctx(t, "login", "nope"), 1)
}
