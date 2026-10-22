package aws_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/aws"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

var _ provider.Provider = (*aws.Provider)(nil)

var fakeAWSBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gctx-fakeaws-")
	if err != nil {
		panic(err)
	}
	fakeAWSBin = filepath.Join(dir, "aws")
	if out, err := exec.Command("go", "build", "-o", fakeAWSBin, "github.com/k0wl0n/gctx/internal/testutil/fakeaws").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakeaws: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type harness struct {
	provider   *aws.Provider
	configFile string
	logPath    string
}

func newHarness(t *testing.T, configBody string, extraEnv ...string) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{configFile: filepath.Join(dir, "config"), logPath: filepath.Join(dir, "calls.log")}
	if configBody != "" {
		if err := os.WriteFile(h.configFile, []byte(configBody), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runner := execrunner.New(fakeAWSBin).WithEnv(append([]string{"FAKE_AWS_LOG=" + h.logPath}, extraEnv...))
	h.provider = aws.New(aws.Options{
		ConfigFile: h.configFile,
		LookPath:   func(string) (string, error) { return fakeAWSBin, nil },
		Getenv:     func(string) string { return "" },
		Runner:     runner,
	})
	return h
}

// calls returns the recorded argument lists.
func (h *harness) calls(t *testing.T) [][]string {
	t.Helper()
	f, err := os.Open(h.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var all [][]string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry struct{ Args []string }
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		all = append(all, entry.Args)
	}
	return all
}

func account(name, profile string) config.Account {
	return config.Account{Name: name, ProjectID: profile, Provider: "aws"}
}

const ssoConfig = "[profile work]\nsso_start_url = https://example.awsapps.com/start\nsso_region = eu-west-1\nregion = eu-west-1\n"

func TestAWSLocalMissingProfile(t *testing.T) {
	h := newHarness(t, ssoConfig)
	local := h.provider.Local(account("prod", "prod-profile"))
	if local.Status != provider.HealthConfigMissing {
		t.Fatalf("status %s, want CONFIG MISSING", local.Status)
	}
	if got := h.provider.Local(account("work", "work")); got.Status != provider.HealthReady || got.Identity != "" {
		t.Fatalf("existing profile: %+v", got)
	}
}

func TestAWSLocalDefaultProfileAndMissingFile(t *testing.T) {
	h := newHarness(t, "[default]\nregion = us-east-1\n")
	if got := h.provider.Local(account("d", "default")); got.Status != provider.HealthReady {
		t.Fatalf("default profile status %s", got.Status)
	}
	missing := newHarness(t, "")
	if got := missing.provider.Local(account("d", "default")); got.Status != provider.HealthConfigMissing {
		t.Fatalf("no config file status %s", got.Status)
	}
}

func TestAWSCheckAuthParsesArn(t *testing.T) {
	h := newHarness(t, ssoConfig, "FAKE_AWS_ARN=arn:aws:sts::1:assumed-role/Dev/me")
	report := h.provider.CheckAuth(context.Background(), []config.Account{account("work", "work")})
	if !report.Credentialed["work"] || report.Identities["work"] != "arn:aws:sts::1:assumed-role/Dev/me" {
		t.Fatalf("report %+v", report)
	}
	want := []string{"sts", "get-caller-identity", "--profile", "work", "--output", "json"}
	if calls := h.calls(t); len(calls) != 1 || strings.Join(calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("calls %v", calls)
	}
}

func TestAWSCheckAuthExpired(t *testing.T) {
	h := newHarness(t, ssoConfig, "FAKE_AWS_STS_FAIL=1")
	report := h.provider.CheckAuth(context.Background(), []config.Account{account("work", "work")})
	if credentialed, present := report.Credentialed["work"]; !present || credentialed {
		t.Fatalf("report %+v, want work present and not credentialed", report)
	}
	if len(report.Identities) != 0 {
		t.Errorf("identities %v", report.Identities)
	}
}

func TestAWSCheckAuthHonoursContext(t *testing.T) {
	h := newHarness(t, ssoConfig, "FAKE_AWS_STS_HANG=1")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	report := h.provider.CheckAuth(ctx, []config.Account{account("work", "work")})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("CheckAuth took %s with a 200ms deadline", elapsed)
	}
	if report.Credentialed["work"] {
		t.Fatalf("report %+v", report)
	}
	if len(report.Warnings) == 0 {
		t.Error("expected a warning when the check could not complete")
	}
}

func TestAWSEnv(t *testing.T) {
	h := newHarness(t, ssoConfig)
	env, err := h.provider.Env(account("work", "work"))
	if err != nil || len(env) != 1 || env[0] != "AWS_PROFILE=work" {
		t.Fatalf("env %v err %v", env, err)
	}
}

func TestAWSLoginUsesProfile(t *testing.T) {
	h := newHarness(t, ssoConfig)
	var events []provider.LoginEventKind
	result, err := h.provider.Login(context.Background(), account("work", "work"), provider.LoginIO{
		Progress: func(e provider.LoginEvent) { events = append(events, e.Kind) },
	})
	if err != nil || result.Account != "work" {
		t.Fatalf("result %+v err %v", result, err)
	}
	calls := h.calls(t)
	if len(calls) != 1 || strings.Join(calls[0], " ") != "sso login --profile work" {
		t.Fatalf("calls %v", calls)
	}
	if len(events) != 2 || events[0] != provider.LoginAuthStarted || events[1] != provider.LoginAuthDone {
		t.Errorf("events %v", events)
	}
}

func TestAWSLoginRejectsNonSSOProfile(t *testing.T) {
	h := newHarness(t, "[profile keys]\nregion = us-east-1\n")
	_, err := h.provider.Login(context.Background(), account("k", "keys"), provider.LoginIO{})
	if err == nil || !strings.Contains(err.Error(), "does not use SSO") || !strings.Contains(err.Error(), "aws configure") {
		t.Fatalf("err %v", err)
	}
	if len(h.calls(t)) != 0 {
		t.Error("aws must not run for a non-SSO profile")
	}
}

func TestAWSMissingCLI(t *testing.T) {
	p := aws.New(aws.Options{LookPath: func(string) (string, error) { return "", errors.New("not found") }})
	if cli := p.CLI(); cli.Found {
		t.Fatalf("CLI %+v, want not found", cli)
	}
	if _, err := p.Create(context.Background(), account("x", "x")); err == nil {
		t.Error("Create without aws CLI should fail")
	}
}

func TestAWSCreateSSOWritesProfileViaConfigure(t *testing.T) {
	h := newHarness(t, "")
	acct := account("work", "work")
	acct.Settings = map[string]string{
		"auth": "sso", "sso_start_url": "https://example.awsapps.com/start",
		"sso_region": "eu-west-1", "region": "eu-central-1",
	}
	outcome, err := h.provider.Create(context.Background(), acct)
	if err != nil || outcome.Target != "work" || !outcome.TargetSet {
		t.Fatalf("outcome %+v err %v", outcome, err)
	}
	var got []string
	for _, call := range h.calls(t) {
		got = append(got, strings.Join(call, " "))
	}
	want := []string{
		"configure set profile.work.sso_start_url https://example.awsapps.com/start",
		"configure set profile.work.sso_region eu-west-1",
		"configure set profile.work.region eu-central-1",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, line := range got {
		lower := strings.ToLower(line)
		for _, secret := range []string{"secret", "token", "access_key", "password"} {
			if strings.Contains(lower, secret) {
				t.Errorf("call %q looks secret-bearing", line)
			}
		}
	}
	if local := h.provider.Local(acct); local.Status != provider.HealthReady {
		t.Errorf("created profile status %s", local.Status)
	}
}

func TestAWSCreateSSORequiresStartURLAndRegion(t *testing.T) {
	h := newHarness(t, "")
	acct := account("work", "work")
	acct.Settings = map[string]string{"auth": "sso"}
	if _, err := h.provider.Create(context.Background(), acct); err == nil || !strings.Contains(err.Error(), "sso_start_url") {
		t.Fatalf("err %v", err)
	}
}

func TestAWSCreateSSORefusesExistingProfile(t *testing.T) {
	h := newHarness(t, ssoConfig)
	acct := account("work", "work")
	acct.Settings = map[string]string{"auth": "sso", "sso_start_url": "https://x/start", "sso_region": "eu-west-1"}
	if _, err := h.provider.Create(context.Background(), acct); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err %v", err)
	}
	if len(h.calls(t)) != 0 {
		t.Error("existing profile must not be modified")
	}
}

func TestAWSCreateProfileMustExist(t *testing.T) {
	h := newHarness(t, ssoConfig)
	acct := account("prod", "prod-profile")
	acct.Settings = map[string]string{"auth": "profile"}
	_, err := h.provider.Create(context.Background(), acct)
	if err == nil || !strings.Contains(err.Error(), "prod-profile") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err %v", err)
	}
	adopted := account("work", "work")
	adopted.Settings = map[string]string{"auth": "profile"}
	if _, err := h.provider.Create(context.Background(), adopted); err != nil {
		t.Fatalf("adopting existing profile: %v", err)
	}
	if len(h.calls(t)) != 0 {
		t.Error("adopting must not run aws")
	}
}

func TestAWSCreateRejectsUnknownAuth(t *testing.T) {
	h := newHarness(t, ssoConfig)
	acct := account("work", "work")
	acct.Settings = map[string]string{"auth": "keys"}
	if _, err := h.provider.Create(context.Background(), acct); err == nil || !strings.Contains(err.Error(), "sso") {
		t.Fatalf("err %v", err)
	}
}

func TestAWSActivateApplyHint(t *testing.T) {
	h := newHarness(t, ssoConfig)
	result, err := h.provider.Activate(context.Background(), account("work", "work"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `eval "$(` + config.AppName + ` env work)"`; result.ApplyHint != want || len(result.Warnings) != 0 || result.Account != "work" {
		t.Fatalf("result %+v, want hint %s", result, want)
	}
	if len(h.calls(t)) != 0 {
		t.Error("Activate must not run aws")
	}
}

func TestAWSValidateTarget(t *testing.T) {
	p := aws.New(aws.Options{})
	for _, ok := range []string{"default", "my.profile_1-x", strings.Repeat("a", 64)} {
		if err := p.ValidateTarget(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "has space", "semi;colon", strings.Repeat("a", 65)} {
		if err := p.ValidateTarget(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestAWSIdentityAndLabels(t *testing.T) {
	p := aws.New(aws.Options{})
	if p.ID() != provider.AWS || p.Label() != "AWS" || p.TargetLabel() != "PROFILE" {
		t.Fatalf("%s %s %s", p.ID(), p.Label(), p.TargetLabel())
	}
}

func TestAWSDetailsAndDelete(t *testing.T) {
	h := newHarness(t, ssoConfig)
	acct := account("work", "work")
	acct.Settings = map[string]string{"auth": "sso", "region": "eu-west-1", "sso_start_url": "https://x/start"}
	fields := map[string]string{}
	for _, f := range h.provider.Details(acct) {
		fields[f.Label] = f.Value
	}
	if fields["Profile"] != "work" || fields["Region"] != "eu-west-1" || fields["Auth"] != "sso" || fields["SSO start URL"] != "https://x/start" {
		t.Fatalf("fields %v", fields)
	}
	result, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{Logout: true})
	if err != nil || result.Account != "work" || len(result.Removed) != 0 {
		t.Fatalf("delete %+v err %v", result, err)
	}
	if data, _ := os.ReadFile(h.configFile); string(data) != ssoConfig {
		t.Error("Delete must not edit the AWS config")
	}
}

func TestAWSCreateSSOWritesAccountAndRoleWhenProvided(t *testing.T) {
	h := newHarness(t, "")
	acct := account("work", "work")
	acct.Settings = map[string]string{
		"auth": "sso", "sso_start_url": "https://example.awsapps.com/start", "sso_region": "eu-west-1",
		"sso_account_id": "123456789012", "sso_role_name": "Developer",
	}
	if _, err := h.provider.Create(context.Background(), acct); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, call := range h.calls(t) {
		got = append(got, strings.Join(call, " "))
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"configure set profile.work.sso_account_id 123456789012",
		"configure set profile.work.sso_role_name Developer",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing call %q in:\n%s", want, joined)
		}
	}
	for _, secret := range []string{"secret", "token", "access_key", "password"} {
		if strings.Contains(strings.ToLower(joined), secret) {
			t.Errorf("calls look secret-bearing: %s", joined)
		}
	}
}

func TestAWSLoginWarnsWhenAccountOrRoleMissing(t *testing.T) {
	h := newHarness(t, ssoConfig)
	result, err := h.provider.Login(context.Background(), account("work", "work"), provider.LoginIO{})
	if err != nil || len(result.Warnings) != 1 {
		t.Fatalf("result %+v err %v", result, err)
	}
	message := result.Warnings[0].Message
	for _, want := range []string{
		"sso_account_id", "sso_role_name",
		"aws configure set profile.work.sso_account_id ACCOUNT_ID",
		"aws configure set profile.work.sso_role_name ROLE_NAME",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("warning %q lacks %q", message, want)
		}
	}
	if result.Warnings[0].Account != "work" {
		t.Errorf("warning account %q", result.Warnings[0].Account)
	}
}

func TestAWSLoginNoWarningWhenComplete(t *testing.T) {
	h := newHarness(t, ssoConfig+"sso_account_id = 123456789012\nsso_role_name = Developer\n")
	result, err := h.provider.Login(context.Background(), account("work", "work"), provider.LoginIO{})
	if err != nil || len(result.Warnings) != 0 {
		t.Fatalf("result %+v err %v", result, err)
	}
}
