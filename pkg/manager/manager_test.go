package manager_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/internal/testutil"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
)

func newManager(t *testing.T, env *testutil.Env) *manager.Manager {
	t.Helper()
	m, err := manager.New()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func warningCodes(warnings []manager.Warning) []manager.WarningCode {
	var codes []manager.WarningCode
	for _, w := range warnings {
		codes = append(codes, w.Code)
	}
	return codes
}

func hasWarning(warnings []manager.Warning, code manager.WarningCode) bool {
	for _, w := range warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

func status(t *testing.T, m *manager.Manager, name string) manager.AccountStatus {
	t.Helper()
	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	account, ok := overview.Account(name)
	if !ok {
		t.Fatalf("account %s missing", name)
	}
	return account
}

func TestHealthStatuses(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("ready", "p1", testutil.SeedOptions{})
	env.SeedAccount("noadc", "p2", testutil.SeedOptions{NoADC: true})
	env.SeedAccount("noconfig", "p3", testutil.SeedOptions{NoGcloudConfig: true})
	env.SeedAccount("mismatch", "p4", testutil.SeedOptions{GcloudProject: "other"})
	env.SeedAccount("both", "p5", testutil.SeedOptions{NoADC: true, GcloudProject: "other"})
	env.SeedAccount("corrupt", "p6", testutil.SeedOptions{})
	env.WriteFile(env.StoredADCPath("corrupt"), []byte(`{"refresh_token": "1//abc`), 0600)
	m := newManager(t, env)

	tests := map[string]manager.Health{
		"ready":    manager.HealthReady,
		"noadc":    manager.HealthADCMissing,
		"noconfig": manager.HealthConfigMissing,
		"mismatch": manager.HealthProjectMismatch,
		"both":     manager.HealthADCMissing,
		"corrupt":  manager.HealthError,
	}
	for name, want := range tests {
		if got := status(t, m, name).Status; got != want {
			t.Errorf("%s: status %s, want %s", name, got, want)
		}
	}
	both := status(t, m, "both")
	if !both.HasFinding(manager.HealthADCMissing) || !both.HasFinding(manager.HealthProjectMismatch) {
		t.Errorf("findings %+v", both.Findings)
	}
	corrupt := status(t, m, "corrupt")
	if !strings.Contains(corrupt.Findings[0].Message, "saved ADC file is invalid") || strings.Contains(corrupt.Findings[0].Message, "1//abc") {
		t.Errorf("message %q", corrupt.Findings[0].Message)
	}
}

func TestHealthAuthRequiredWithoutAccount(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("x", "p1", testutil.SeedOptions{})
	env.WriteFile(env.GcloudConfigPath("x-config"), []byte("[core]\nproject = p1\n"), 0644)
	if got := status(t, newManager(t, env), "x").Status; got != manager.HealthAuthRequired {
		t.Errorf("status %s", got)
	}
}

func TestHealthCorruptGcloudConfigIsError(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("x", "p1", testutil.SeedOptions{})
	if err := os.Remove(env.GcloudConfigPath("x-config")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(env.GcloudConfigPath("x-config"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := status(t, newManager(t, env), "x").Status; got != manager.HealthError {
		t.Errorf("status %s", got)
	}
}

func TestOverviewHonoursCloudsdkConfig(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	custom := filepath.Join(env.Root, "custom-gcloud")
	if err := os.Rename(env.GcloudDir, custom); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLOUDSDK_CONFIG", custom)
	if got := status(t, newManager(t, env), "work").Status; got != manager.HealthReady {
		t.Errorf("status %s", got)
	}
}

func TestOverviewHasNoSideEffects(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	env.SetActive("work")
	env.WriteFile(env.DefaultADCPath(), testutil.ADCJSON("work"), 0600)
	m := newManager(t, env)

	files := []string{env.ConfigPath(), env.StoredADCPath("work"), env.StoredADCPath("sandbox"),
		env.DefaultADCPath(), env.GcloudConfigPath("work-config"), env.GcloudConfigPath("sandbox-config")}
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	before := map[string]time.Time{}
	for _, f := range files {
		os.Chtimes(f, old, old)
		before[f] = old
	}
	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.DefaultADC != manager.ADCActive {
		t.Errorf("default ADC %s", overview.DefaultADC)
	}
	if calls := env.Invocations(); len(calls) != 0 {
		t.Errorf("gcloud invoked: %v", calls)
	}
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil || !info.ModTime().Equal(before[f]) {
			t.Errorf("%s changed", f)
		}
	}
}

func TestDefaultADCIndicator(t *testing.T) {
	env := testutil.NewEnv(t)
	m := newManager(t, env)
	overview, _ := m.Overview()
	if overview.DefaultADC != manager.ADCNone {
		t.Errorf("none: %s", overview.DefaultADC)
	}
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.WriteFile(env.DefaultADCPath(), testutil.ADCJSON("work"), 0600)
	overview, _ = m.Overview()
	if overview.DefaultADC != manager.ADCUnknown {
		t.Errorf("unknown: %s", overview.DefaultADC)
	}
	env.SetActive("work")
	env.WriteFile(env.DefaultADCPath(), testutil.ADCJSON("other"), 0600)
	overview, _ = m.Overview()
	if overview.DefaultADC != manager.ADCDiffers {
		t.Errorf("differs: %s", overview.DefaultADC)
	}
}

func TestCredentialCheck(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("ok", "p1", testutil.SeedOptions{})
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{NoCredential: true})
	m := newManager(t, env)

	if got := status(t, m, "sandbox").Status; got != manager.HealthReady {
		t.Errorf("before check: %s", got)
	}
	report, err := m.CheckCredentials(context.Background())
	if err != nil || len(report.Warnings) != 0 {
		t.Fatalf("%v %+v", err, report)
	}
	if got := status(t, m, "sandbox").Status; got != manager.HealthAuthRequired {
		t.Errorf("sandbox: %s", got)
	}
	if got := status(t, m, "ok").Status; got != manager.HealthReady {
		t.Errorf("ok: %s", got)
	}
	for _, call := range env.Invocations() {
		if strings.Contains(call.Line(), "print-") {
			t.Errorf("token command %q", call.Line())
		}
	}
}

func TestCredentialCheckGcloudMissing(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("ok", "p1", testutil.SeedOptions{})
	env.HideGcloud()
	m := newManager(t, env)
	report, err := m.CheckCredentials(context.Background())
	if err != nil || !hasWarning(report.Warnings, manager.WarnGcloudNotFound) {
		t.Fatalf("%v %+v", err, report)
	}
	if got := status(t, m, "ok").Status; got != manager.HealthReady {
		t.Errorf("status %s", got)
	}
}

func TestCredentialCheckFailureIsWarning(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("ok", "p1", testutil.SeedOptions{})
	t.Setenv("FAKE_GCLOUD_FAIL", "auth-list:1:boom")
	report, err := newManager(t, env).CheckCredentials(context.Background())
	if err != nil || len(report.Warnings) != 1 || report.Warnings[0].Code != manager.WarnCredentialCheck {
		t.Fatalf("%v %+v", err, report)
	}
}

func TestCreateValidationBeforeSideEffects(t *testing.T) {
	env := testutil.NewEnv(t)
	m := newManager(t, env)
	for _, name := range []string{"Work", "../evil", ""} {
		if _, err := m.Create(context.Background(), name, "demo-project"); !errors.Is(err, manager.ErrInvalidName) {
			t.Errorf("%q: %v", name, err)
		}
	}
	if len(env.Invocations()) != 0 {
		t.Error("gcloud invoked")
	}
	if _, err := os.Stat(env.ConfigPath()); !os.IsNotExist(err) {
		t.Error("config.json written")
	}
}

func TestCreateDuplicateChecksFirst(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("demo", "p1", testutil.SeedOptions{})
	_, err := newManager(t, env).Create(context.Background(), "demo", "demo-project")
	if !errors.Is(err, manager.ErrAccountExists) || err.Error() != "account 'demo' already exists" {
		t.Fatalf("err %v", err)
	}
	if len(env.Invocations()) != 0 {
		t.Error("gcloud invoked")
	}
}

func TestCreateReusedConfigAndProjectFailure(t *testing.T) {
	env := testutil.NewEnv(t)
	env.WriteFile(env.GcloudConfigPath("demo-config"), []byte("[core]\n"), 0644)
	t.Setenv("FAKE_GCLOUD_FAIL", "config-set:1:ERROR: Reauthentication required.")
	result, err := newManager(t, env).Create(context.Background(), "demo", "demo-project")
	if err != nil {
		t.Fatal(err)
	}
	codes := warningCodes(result.Warnings)
	if len(codes) != 2 || codes[0] != manager.WarnConfigReused || codes[1] != manager.WarnSetProjectFailed || result.ProjectSet {
		t.Errorf("warnings %v", codes)
	}
	if env.ReadConfig().Accounts["demo"] == nil {
		t.Error("account not recorded")
	}
}

func loginIO(events *[]manager.LoginEvent) manager.LoginIO {
	return manager.LoginIO{
		Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
		Progress: func(e manager.LoginEvent) { *events = append(*events, e) },
	}
}

func TestLoginSuccess(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{NoADC: true})
	m := newManager(t, env)

	var events []manager.LoginEvent
	start := time.Now()
	result, err := m.Login(context.Background(), "work", loginIO(&events))
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("login waited")
	}
	var kinds []manager.LoginEventKind
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	want := []manager.LoginEventKind{manager.LoginAuthStarted, manager.LoginAuthDone, manager.LoginADCStarted, manager.LoginVerifyingADC}
	if len(kinds) != len(want) {
		t.Fatalf("events %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("event %d: %s", i, kinds[i])
		}
	}
	if !result.ADCSaved || result.Email != "work@example.com" {
		t.Errorf("result %+v", result)
	}
	for _, call := range env.Invocations() {
		if call.Verb == "config-get-value" && call.ActiveConfig != "work-config" {
			t.Errorf("email read with %q", call.ActiveConfig)
		}
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(env.StoredADCPath("work"))
		if info.Mode().Perm() != 0600 {
			t.Errorf("mode %v", info.Mode().Perm())
		}
	}
}

func TestLoginStepErrors(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	stored, _ := os.ReadFile(env.StoredADCPath("work"))
	m := newManager(t, env)

	for _, tt := range []struct {
		fail, step string
		loggedIn   bool
	}{
		{"auth-login:1", manager.StepAuthLogin, false},
		{"auth-adc-login:1", manager.StepADCLogin, true},
	} {
		t.Setenv("FAKE_GCLOUD_FAIL", tt.fail)
		var events []manager.LoginEvent
		result, err := m.Login(context.Background(), "work", loginIO(&events))
		var stepErr *gcloud.StepError
		if !errors.As(err, &stepErr) || stepErr.Step != tt.step || result.GcloudLoggedIn != tt.loggedIn || result.ADCSaved {
			t.Errorf("%s: err %v result %+v", tt.fail, err, result)
		}
		after, _ := os.ReadFile(env.StoredADCPath("work"))
		if !bytes.Equal(after, stored) || env.ReadConfig().Accounts["work"].ADCPath != env.StoredADCPath("work") {
			t.Errorf("%s: stored ADC modified", tt.fail)
		}
	}
}

func TestLoginADCNotRefreshed(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{NoADC: true})
	env.WriteFile(env.DefaultADCPath(), testutil.ADCJSON("existing"), 0600)
	t.Setenv("FAKE_GCLOUD_ADC_UNCHANGED", "1")
	var events []manager.LoginEvent
	result, err := newManager(t, env).Login(context.Background(), "work", loginIO(&events))
	if err != nil || !hasWarning(result.Warnings, manager.WarnADCNotRefreshed) || !result.ADCSaved {
		t.Fatalf("%v %+v", err, result)
	}
}

func TestSwitchWarnings(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{NoADC: true})
	m := newManager(t, env)
	result, err := m.Switch(context.Background(), "sandbox")
	if err != nil || !hasWarning(result.Warnings, manager.WarnADCMissing) {
		t.Fatalf("%v %+v", err, result)
	}

	t.Setenv("FAKE_GCLOUD_FAIL", "config-set:1:ERROR: Reauthentication required.")
	result, err = m.Switch(context.Background(), "sandbox")
	if err != nil || !hasWarning(result.Warnings, manager.WarnReauthRequired) {
		t.Fatalf("%v %+v", err, result)
	}
	if !status(t, m, "sandbox").HasFinding(manager.HealthAuthRequired) {
		t.Error("session AUTH REQUIRED not set")
	}
}

func TestSwitchActivationFailureKeepsActive(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	env.SetActive("work")
	t.Setenv("FAKE_GCLOUD_FAIL", "configurations-activate:1")
	if _, err := newManager(t, env).Switch(context.Background(), "sandbox"); err == nil {
		t.Fatal("expected error")
	}
	if env.ReadConfig().ActiveAccount != "work" {
		t.Error("active changed")
	}
}

func TestSwitchPinsConfigDespiteParentEnv(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("personal", "p9", testutil.SeedOptions{})
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "personal-config")
	if _, err := newManager(t, env).Switch(context.Background(), "work"); err != nil {
		t.Fatal(err)
	}
	if env.GcloudProperty("work-config", "project") != "p1" || env.GcloudProperty("personal-config", "project") != "p9" {
		t.Error("project set on the wrong configuration")
	}
}

func TestSwitchPreservesExternalChanges(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	m := newManager(t, env)
	m.Overview()
	env.SeedAccount("demo", "p3", testutil.SeedOptions{})
	if _, err := m.Switch(context.Background(), "sandbox"); err != nil {
		t.Fatal(err)
	}
	cfg := env.ReadConfig()
	if len(cfg.Accounts) != 3 || cfg.ActiveAccount != "sandbox" {
		t.Errorf("config %+v", cfg)
	}
}

func TestSaveUsesAccountConfigEmail(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{NoADC: true})
	env.SetGcloudActiveConfig("work-config")
	env.WriteFile(env.DefaultADCPath(), testutil.ADCJSON("current"), 0600)
	result, err := newManager(t, env).SaveCredentials(context.Background(), "sandbox")
	if err != nil || result.Email != "sandbox@example.com" {
		t.Fatalf("%v %+v", err, result)
	}
}

func TestSaveWithoutDefaultADC(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{NoADC: true})
	_, err := newManager(t, env).SaveCredentials(context.Background(), "work")
	if err == nil || !strings.HasPrefix(err.Error(), "no ADC found at ") {
		t.Fatalf("err %v", err)
	}
	if _, statErr := os.Stat(env.StoredADCPath("work")); !os.IsNotExist(statErr) {
		t.Error("file written")
	}
}

func TestDelete(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("sandbox", "p2", testutil.SeedOptions{})
	env.SetGcloudActiveConfig("work-config")
	env.WriteFile(env.DefaultADCPath(), testutil.ADCJSON("work"), 0600)
	m := newManager(t, env)

	result, err := m.Delete(context.Background(), "sandbox", manager.DeleteOptions{})
	if err != nil || len(result.Warnings) != 0 {
		t.Fatalf("%v %+v", err, result)
	}
	if _, statErr := os.Stat(env.StoredADCPath("sandbox")); !os.IsNotExist(statErr) {
		t.Error("stored ADC remains")
	}
	if _, statErr := os.Stat(env.GcloudConfigPath("sandbox-config")); statErr != nil {
		t.Error("gcloud config removed by default")
	}
	if _, statErr := os.Stat(env.DefaultADCPath()); statErr != nil {
		t.Error("default ADC deleted")
	}
}

func TestDeleteConfigFailureStillRemovesAccount(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SetGcloudActiveConfig("work-config")
	result, err := newManager(t, env).Delete(context.Background(), "work", manager.DeleteOptions{GcloudConfig: true})
	if err != nil || !hasWarning(result.Warnings, manager.WarnConfigDeleteFailed) {
		t.Fatalf("%v %+v", err, result)
	}
	if len(env.ReadConfig().Accounts) != 0 {
		t.Error("account remains")
	}
}

func TestDeleteTamperedADCPath(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("x", "p1", testutil.SeedOptions{})
	victim := filepath.Join(env.Root, "id_ed25519")
	env.WriteFile(victim, []byte("private"), 0600)
	cfg := env.ReadConfig()
	cfg.Accounts["x"].ADCPath = victim
	env.WriteConfig(cfg)

	result, err := newManager(t, env).Delete(context.Background(), "x", manager.DeleteOptions{})
	if err != nil || !hasWarning(result.Warnings, manager.WarnADCPathOutsideStore) {
		t.Fatalf("%v %+v", err, result)
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Error("file outside the store was deleted")
	}
	if _, statErr := os.Stat(env.StoredADCPath("x")); !os.IsNotExist(statErr) {
		t.Error("canonical ADC remains")
	}
}

func TestDeleteDoesNotResurrectCredentials(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("x", "p1", testutil.SeedOptions{})
	m := newManager(t, env)
	if _, err := m.Delete(context.Background(), "x", manager.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), "x", "new-project"); err != nil {
		t.Fatal(err)
	}
	if status(t, m, "x").ADCExists {
		t.Error("stale ADC attached to the new account")
	}
}

func TestRunAndShellEnvironment(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	env.SeedAccount("personal", "p9", testutil.SeedOptions{})
	env.SetActive("work")
	env.WriteFile(env.DefaultADCPath(), testutil.ADCJSON("work"), 0600)
	defaultBefore, _ := os.ReadFile(env.DefaultADCPath())
	t.Setenv("SHELL", env.FakeShell())
	m := newManager(t, env)

	run, err := m.RunCommand("personal", []string{"config", "get-value", "project"})
	if err != nil {
		t.Fatal(err)
	}
	if got := run.Args[1:]; strings.Join(got, " ") != "config get-value project" {
		t.Errorf("args %v", run.Args)
	}
	if !envContains(run.Env, "CLOUDSDK_ACTIVE_CONFIG_NAME=personal-config") ||
		!envContains(run.Env, "GOOGLE_APPLICATION_CREDENTIALS="+env.StoredADCPath("personal")) {
		t.Errorf("env %v", run.Env)
	}

	shell, err := m.ShellCommand("personal")
	if err != nil {
		t.Fatal(err)
	}
	if shell.Path != env.FakeShell() || !envContains(shell.Env, "GCTX_ACCOUNT=personal") ||
		!envContains(shell.Env, "CLOUDSDK_ACTIVE_CONFIG_NAME=personal-config") {
		t.Errorf("shell %s env %v", shell.Path, shell.Env)
	}

	if err := run.Run(); err != nil {
		t.Fatal(err)
	}
	defaultAfter, _ := os.ReadFile(env.DefaultADCPath())
	if !bytes.Equal(defaultBefore, defaultAfter) || env.ReadConfig().ActiveAccount != "work" {
		t.Error("global state changed")
	}
}

func envContains(env []string, entry string) bool {
	for _, e := range env {
		if e == entry {
			return true
		}
	}
	return false
}

func TestResolveShell(t *testing.T) {
	look := func(found ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, f := range found {
				if f == name {
					return `C:\bin\` + name, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	env := func(vars map[string]string) func(string) string {
		return func(key string) string { return vars[key] }
	}
	tests := []struct {
		name string
		goos string
		vars map[string]string
		look func(string) (string, error)
		want string
	}{
		{"unix SHELL", "linux", map[string]string{"SHELL": "/usr/bin/fish"}, look(), "/usr/bin/fish"},
		{"unix default", "darwin", nil, look(), "/bin/sh"},
		{"windows unresolvable SHELL", "windows", map[string]string{"SHELL": "/usr/bin/bash"}, look("pwsh.exe"), `C:\bin\pwsh.exe`},
		{"windows powershell", "windows", nil, look("powershell.exe"), `C:\bin\powershell.exe`},
		{"windows comspec", "windows", map[string]string{"COMSPEC": `C:\Windows\cmd.exe`}, look(), `C:\Windows\cmd.exe`},
		{"windows cmd", "windows", nil, look(), "cmd.exe"},
		{"windows resolvable SHELL", "windows", map[string]string{"SHELL": "bash"}, look("bash", "pwsh.exe"), `C:\bin\bash`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := manager.ResolveShell(tt.goos, env(tt.vars), tt.look); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNonInteractiveGcloudCalls(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	if _, err := newManager(t, env).Switch(context.Background(), "work"); err != nil {
		t.Fatal(err)
	}
	for _, call := range env.Invocations() {
		if !call.StdinDevNull || call.DisablePrompts != "1" {
			t.Errorf("%q: %+v", call.Line(), call)
		}
	}
}

func TestOperationsReportMissingAccount(t *testing.T) {
	env := testutil.NewEnv(t)
	m := newManager(t, env)
	if _, err := m.Info("nope"); !errors.Is(err, manager.ErrAccountNotFound) || err.Error() != "account 'nope' not found" {
		t.Errorf("info: %v", err)
	}
	if _, err := m.Switch(context.Background(), "nope"); !errors.Is(err, manager.ErrAccountNotFound) {
		t.Errorf("switch: %v", err)
	}
	if _, err := m.RunCommand("nope", nil); !errors.Is(err, manager.ErrAccountNotFound) {
		t.Errorf("run: %v", err)
	}
}
