package testutil

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func runGcloud(t *testing.T, env *Env, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(env.GcloudBin(), args...)
	out, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), exitErr.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(out), 0
}

func TestFakeGcloudConfigurations(t *testing.T) {
	env := NewEnv(t)

	if _, code := runGcloud(t, env, "config", "configurations", "create", "work-config"); code != 0 {
		t.Fatalf("create exit %d", code)
	}
	if got := env.GcloudActiveConfig(); got != "work-config" {
		t.Fatalf("create should activate, active=%q", got)
	}
	out, code := runGcloud(t, env, "config", "configurations", "create", "work-config")
	if code == 0 || !strings.Contains(out, "already exists") {
		t.Fatalf("duplicate create: code=%d out=%q", code, out)
	}

	runGcloud(t, env, "config", "set", "project", "p1")
	if got := env.GcloudProperty("work-config", "project"); got != "p1" {
		t.Fatalf("project=%q", got)
	}

	out, code = runGcloud(t, env, "config", "configurations", "delete", "work-config", "--quiet")
	if code == 0 || !strings.Contains(out, "currently active") {
		t.Fatalf("deleting active config should fail: %d %q", code, out)
	}
	runGcloud(t, env, "config", "configurations", "create", "other-config")
	if _, code := runGcloud(t, env, "config", "configurations", "delete", "work-config", "--quiet"); code != 0 {
		t.Fatalf("delete exit %d", code)
	}
	if _, err := os.Stat(env.GcloudConfigPath("work-config")); !os.IsNotExist(err) {
		t.Fatal("config file should be gone")
	}
}

func TestFakeGcloudAuth(t *testing.T) {
	env := NewEnv(t)
	runGcloud(t, env, "config", "configurations", "create", "work-config")
	runGcloud(t, env, "auth", "login")
	if got := env.GcloudProperty("work-config", "account"); got != "work@example.com" {
		t.Fatalf("account=%q", got)
	}
	out, _ := runGcloud(t, env, "config", "get-value", "account")
	if strings.TrimSpace(out) != "work@example.com" {
		t.Fatalf("get-value account=%q", out)
	}

	runGcloud(t, env, "auth", "application-default", "login")
	data, err := os.ReadFile(env.DefaultADCPath())
	if err != nil || !json.Valid(data) {
		t.Fatalf("ADC not written: %v", err)
	}

	out, _ = runGcloud(t, env, "auth", "list", "--format=json")
	if !strings.Contains(out, `"work@example.com"`) {
		t.Fatalf("auth list=%q", out)
	}
}

func TestFakeGcloudInjectionAndLog(t *testing.T) {
	env := NewEnv(t)
	t.Setenv("FAKE_GCLOUD_FAIL", "auth-login:7:Reauthentication required")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "x-config")
	out, code := runGcloud(t, env, "auth", "login")
	if code != 7 || !strings.Contains(out, "Reauthentication required") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	calls := env.Invocations()
	if len(calls) != 1 || calls[0].Verb != "auth-login" || calls[0].ActiveConfig != "x-config" {
		t.Fatalf("calls=%+v", calls)
	}
}

func TestFakeShell(t *testing.T) {
	env := NewEnv(t)
	t.Setenv("GCTX_ACCOUNT", "personal")
	t.Setenv("FAKE_SHELL_EXIT", "3")
	out, err := exec.Command(env.FakeShell()).Output()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 3 {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(out), "GCTX_ACCOUNT=personal") {
		t.Fatalf("out=%q", out)
	}
}

func TestSeedAccount(t *testing.T) {
	env := NewEnv(t)
	env.SeedAccount("work", "p1", SeedOptions{})
	env.SeedAccount("sandbox", "p2", SeedOptions{NoADC: true})
	env.SetActive("work")

	cfg := env.ReadConfig()
	if cfg.ActiveAccount != "work" || len(cfg.Accounts) != 2 {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.Accounts["sandbox"].ADCPath != "" {
		t.Fatal("sandbox should have no ADC")
	}
	if env.GcloudProperty("sandbox-config", "project") != "p2" {
		t.Fatal("gcloud config not seeded")
	}
}
