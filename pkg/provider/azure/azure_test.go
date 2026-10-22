package azure_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/azure"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

var (
	_ provider.Provider       = (*azure.Provider)(nil)
	_ provider.StreamingLogin = (*azure.Provider)(nil)
)

const (
	subscription = "12345678-1234-1234-1234-123456789abc"
	tokenMarker  = "FAKE-AZ-SECRET-ACCESS-TOKEN"
)

var fakeAZBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gctx-fakeaz-")
	if err != nil {
		panic(err)
	}
	fakeAZBin = filepath.Join(dir, "az")
	if out, err := exec.Command("go", "build", "-o", fakeAZBin, "github.com/k0wl0n/gctx/internal/testutil/fakeaz").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakeaz: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type harness struct {
	provider *azure.Provider
	root     string
	logPath  string
}

func newHarness(t *testing.T, extraEnv ...string) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{root: filepath.Join(dir, "azure"), logPath: filepath.Join(dir, "calls.log")}
	if err := os.MkdirAll(h.root, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := execrunner.New(fakeAZBin).WithEnv(append([]string{"FAKE_AZ_LOG=" + h.logPath}, extraEnv...))
	h.provider = azure.New(azure.Options{
		ConfigRoot: h.root,
		LookPath:   func(string) (string, error) { return fakeAZBin, nil },
		Getenv:     func(string) string { return "" },
		Runner:     runner,
	})
	return h
}

type call struct {
	Args         []string `json:"args"`
	ConfigDir    string   `json:"azure_config_dir"`
	TokenEmitted bool     `json:"token_emitted"`
}

func (h *harness) calls(t *testing.T) []call {
	t.Helper()
	f, err := os.Open(h.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var all []call
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry call
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		all = append(all, entry)
	}
	return all
}

func account(name string) config.Account {
	return config.Account{Name: name, ProjectID: subscription, Provider: "azure", Settings: map[string]string{}}
}

// created returns an account whose isolated directory exists.
func (h *harness) created(t *testing.T, name string) config.Account {
	t.Helper()
	acct := account(name)
	outcome, err := h.provider.Create(context.Background(), acct)
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(acct.Settings, outcome.Settings)
	return acct
}

func TestAzureValidateTargetRejectsNonGUID(t *testing.T) {
	h := newHarness(t)
	if err := h.provider.ValidateTarget(subscription); err != nil {
		t.Fatalf("valid GUID rejected: %v", err)
	}
	for _, bad := range []string{"", "my-subscription", "12345678-1234-1234-1234-123456789abcd", "1234567812341234123412345678abcd", "gggggggg-1234-1234-1234-123456789abc"} {
		if err := h.provider.ValidateTarget(bad); err == nil {
			t.Errorf("ValidateTarget(%q) accepted", bad)
		}
	}
}

func TestAzureCreateMakesPrivateDir(t *testing.T) {
	h := newHarness(t)
	acct := account("work")
	acct.Settings["tenant"] = "contoso.onmicrosoft.com"
	outcome, err := h.provider.Create(context.Background(), acct)
	if err != nil || !outcome.TargetSet {
		t.Fatalf("outcome %+v err %v", outcome, err)
	}
	want := filepath.Join(h.root, "work")
	info, err := os.Stat(want)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir %v mode %v err %v", want, info, err)
	}
	if outcome.Settings["dir"] != want || outcome.Settings["tenant"] != "contoso.onmicrosoft.com" {
		t.Errorf("outcome settings %v, want dir %q and tenant", outcome.Settings, want)
	}
	if _, mutated := acct.Settings["dir"]; mutated {
		t.Error("Create must not mutate the caller's settings")
	}
	if len(h.calls(t)) != 0 {
		t.Error("create must not run az")
	}
}

func TestAzureCreateRejectsEscapingName(t *testing.T) {
	h := newHarness(t)
	for _, name := range []string{"..", "../evil", "a/b", `a\b`, ".", ""} {
		if _, err := h.provider.Create(context.Background(), account(name)); err == nil {
			t.Errorf("Create(%q) accepted", name)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(h.root))
	for _, entry := range entries {
		if entry.Name() == "evil" {
			t.Error("directory created outside the root")
		}
	}
}

func TestAzureEnvIsolatesPerAccount(t *testing.T) {
	h := newHarness(t)
	one, two := h.created(t, "one"), h.created(t, "two")
	envOne, err := h.provider.Env(one)
	if err != nil {
		t.Fatal(err)
	}
	envTwo, err := h.provider.Env(two)
	if err != nil {
		t.Fatal(err)
	}
	if len(envOne) != 1 || envOne[0] != "AZURE_CONFIG_DIR="+filepath.Join(h.root, "one") || envOne[0] == envTwo[0] {
		t.Fatalf("env %v / %v", envOne, envTwo)
	}
	fallback, err := h.provider.Env(account("legacy"))
	if err != nil || fallback[0] != "AZURE_CONFIG_DIR="+filepath.Join(h.root, "legacy") {
		t.Fatalf("fallback env %v err %v", fallback, err)
	}
	tampered := account("evil")
	tampered.Settings["dir"] = t.TempDir()
	if _, err := h.provider.Env(tampered); err == nil {
		t.Error("Env accepted a dir outside the root")
	}
}

func TestAzureCheckAuthExpired(t *testing.T) {
	h := newHarness(t, "FAKE_AZ_TOKEN_FAIL=1")
	acct := h.created(t, "work")
	report := h.provider.CheckAuth(context.Background(), []config.Account{acct})
	if result, present := report.Results["work"]; !present || result.Credentialed {
		t.Fatalf("report %+v, want work present and not credentialed", report)
	}
}

func TestAzureCheckAuthNetworkErrorIsUnverified(t *testing.T) {
	h := newHarness(t, "FAKE_AZ_TOKEN_NETFAIL=1")
	acct := h.created(t, "work")
	report := h.provider.CheckAuth(context.Background(), []config.Account{acct})
	if result := report.Results["work"]; result.Credentialed || result.Err == nil {
		t.Fatalf("result %+v, want not credentialed with Err", result)
	}
}

func TestAzureCheckAuthExpiredStillAuthRequired(t *testing.T) {
	h := newHarness(t, "FAKE_AZ_TOKEN_FAIL=1")
	acct := h.created(t, "work")
	report := h.provider.CheckAuth(context.Background(), []config.Account{acct})
	if result, present := report.Results["work"]; !present || result.Credentialed || result.Err != nil {
		t.Fatalf("result %+v, want a not-credentialed verdict", result)
	}
}

func TestAzureCheckAuthCredentialed(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	report := h.provider.CheckAuth(context.Background(), []config.Account{acct})
	if !report.Results["work"].Credentialed {
		t.Fatalf("report %+v", report)
	}
	calls := h.calls(t)
	if len(calls) != 1 || calls[0].ConfigDir != filepath.Join(h.root, "work") {
		t.Fatalf("calls %+v", calls)
	}
}

func TestAzureCheckAuthNeverRequestsToken(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	var out bytes.Buffer
	if _, err := h.provider.Login(context.Background(), acct, provider.LoginIO{Stdout: &out, Stderr: &out}); err != nil {
		t.Fatal(err)
	}
	report := h.provider.CheckAuth(context.Background(), []config.Account{acct})

	tokenCalls := 0
	for _, c := range h.calls(t) {
		if c.TokenEmitted {
			t.Errorf("fake emitted a token for %v", c.Args)
		}
		if strings.Join(c.Args[:min(2, len(c.Args))], " ") == "account get-access-token" {
			tokenCalls++
			if !strings.Contains(strings.Join(c.Args, " "), "--query expiresOn") {
				t.Errorf("get-access-token without --query: %v", c.Args)
			}
		}
	}
	if tokenCalls != 2 { // login's verify step and the check
		t.Fatalf("token calls %d", tokenCalls)
	}
	recorded, _ := os.ReadFile(h.logPath)
	reportJSON, _ := json.Marshal(report)
	for name, text := range map[string]string{"log": string(recorded), "report": string(reportJSON), "login output": out.String()} {
		if strings.Contains(text, tokenMarker) {
			t.Errorf("%s contains the access token", name)
		}
	}
}

func TestAzureCheckAuthHonoursContext(t *testing.T) {
	h := newHarness(t, "FAKE_AZ_TOKEN_HANG=1")
	acct := h.created(t, "work")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	report := h.provider.CheckAuth(ctx, []config.Account{acct})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("CheckAuth took %s with a 200ms deadline", elapsed)
	}
	if report.Results["work"].Credentialed || len(report.Warnings) == 0 {
		t.Fatalf("report %+v", report)
	}
}

func TestAzureDeleteRefusesPathOutsideRoot(t *testing.T) {
	h := newHarness(t)
	outside := t.TempDir()
	marker := filepath.Join(outside, "keep")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	acct := account("evil")
	acct.Settings["dir"] = outside
	if _, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{}); err == nil {
		t.Fatal("Delete accepted a dir outside the root")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("outside file removed: %v", err)
	}

	acct.Settings["dir"] = h.root
	if _, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{}); err == nil {
		t.Fatal("Delete accepted the root itself")
	}
	if _, err := os.Stat(h.root); err != nil {
		t.Fatalf("root removed: %v", err)
	}
}

func TestAzureDeleteRefusesSymlinkEscape(t *testing.T) {
	h := newHarness(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(h.root, "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	acct := account("link")
	if _, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{}); err == nil {
		t.Fatal("Delete followed a symlink out of the root")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("symlink target removed: %v", err)
	}
}

func TestAzureDeleteRemovesIsolatedDir(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	dir := acct.Settings["dir"]
	result, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{RemoveDir: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dir still exists: %v", err)
	}
	if len(result.Removed) != 1 || result.Removed[0] != dir {
		t.Errorf("removed %v", result.Removed)
	}
}

func TestAzureLoginSetsSubscription(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	acct.Settings["tenant"] = "contoso.onmicrosoft.com"
	var events []provider.LoginEventKind
	var out bytes.Buffer
	result, err := h.provider.Login(context.Background(), acct, provider.LoginIO{
		Stdout: &out,
		Stderr: &out,
		Progress: func(e provider.LoginEvent) {
			if e.Kind == provider.LoginAuthStarted || e.Kind == provider.LoginAuthDone {
				events = append(events, e.Kind)
			}
		},
	})
	if err != nil || result.Account != "work" {
		t.Fatalf("result %+v err %v", result, err)
	}
	calls := h.calls(t)
	if len(calls) != 3 {
		t.Fatalf("calls %+v", calls)
	}
	wantLogin := "login --use-device-code --tenant contoso.onmicrosoft.com"
	wantSet := "account set --subscription " + subscription
	if strings.Join(calls[0].Args, " ") != wantLogin || strings.Join(calls[1].Args, " ") != wantSet {
		t.Fatalf("calls %+v", calls)
	}
	dir := filepath.Join(h.root, "work")
	if calls[0].ConfigDir != dir || calls[1].ConfigDir != dir || calls[2].ConfigDir != dir {
		t.Errorf("config dirs %q %q %q, want %q", calls[0].ConfigDir, calls[1].ConfigDir, calls[2].ConfigDir, dir)
	}
	if strings.Join(calls[2].Args[:2], " ") != "account get-access-token" || calls[2].TokenEmitted {
		t.Errorf("verify call %+v", calls[2])
	}
	if !strings.Contains(out.String(), "devicelogin") {
		t.Errorf("device code instructions not passed through: %q", out.String())
	}
	if len(events) != 2 || events[0] != provider.LoginAuthStarted || events[1] != provider.LoginAuthDone {
		t.Errorf("events %v", events)
	}
}

func TestAzureLoginFailureSkipsSubscription(t *testing.T) {
	h := newHarness(t, "FAKE_AZ_LOGIN_FAIL=1")
	acct := h.created(t, "work")
	if _, err := h.provider.Login(context.Background(), acct, provider.LoginIO{}); err == nil {
		t.Fatal("expected login failure")
	}
	if calls := h.calls(t); len(calls) != 1 {
		t.Fatalf("calls %+v, want only the failed login", calls)
	}
}

func TestAzureLocalStates(t *testing.T) {
	h := newHarness(t)
	acct := account("work")
	if got := h.provider.Local(acct); got.Status != provider.HealthConfigMissing {
		t.Fatalf("missing dir: %s", got.Status)
	}
	acct = h.created(t, "work")
	if got := h.provider.Local(acct); got.Status != provider.HealthAuthRequired {
		t.Fatalf("no profile: %s", got.Status)
	}
	if _, err := h.provider.Login(context.Background(), acct, provider.LoginIO{}); err != nil {
		t.Fatal(err)
	}
	if got := h.provider.Local(acct); got.Status != provider.HealthReady {
		t.Fatalf("after login: %s", got.Status)
	}
}

func TestAzureMissingCLI(t *testing.T) {
	p := azure.New(azure.Options{ConfigRoot: t.TempDir(), LookPath: func(string) (string, error) { return "", errors.New("not found") }})
	if cli := p.CLI(); cli.Found {
		t.Fatalf("CLI %+v, want not found", cli)
	}
	if _, err := p.Login(context.Background(), account("x"), provider.LoginIO{}); !errors.Is(err, azure.ErrCLINotFound) {
		t.Fatalf("login err %v", err)
	}
	report := p.CheckAuth(context.Background(), []config.Account{account("x")})
	if len(report.Warnings) == 0 || report.Results["x"].Credentialed {
		t.Fatalf("report %+v", report)
	}
}

func TestAzureActivateApplyHint(t *testing.T) {
	h := newHarness(t)
	result, err := h.provider.Activate(context.Background(), account("work"))
	if err != nil || result.ApplyHint != `eval "$(gctx env work)"` || len(result.Warnings) != 0 {
		t.Fatalf("result %+v err %v", result, err)
	}
	if len(h.calls(t)) != 0 {
		t.Error("activate must not run az")
	}
}

func TestAzureDetails(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	acct.Settings["tenant"] = "contoso"
	got := map[string]string{}
	for _, field := range h.provider.Details(acct) {
		got[field.Label] = field.Value
	}
	if got["Subscription"] != subscription || got["Tenant"] != "contoso" || got["Config dir"] != filepath.Join(h.root, "work") {
		t.Fatalf("details %v", got)
	}
}

func TestParseDeviceCode(t *testing.T) {
	cases := []struct {
		name, line, url, code string
		ok                    bool
	}{
		{"real az text", "To sign in, use a web browser to open the page https://microsoft.com/devicelogin and enter the code F7K2Q9XM to authenticate.", "https://microsoft.com/devicelogin", "F7K2Q9XM", true},
		{"nine letters", "To sign in, use a web browser to open the page https://microsoft.com/devicelogin and enter the code FDMRYGZTK to authenticate.", "https://microsoft.com/devicelogin", "FDMRYGZTK", true},
		{"ansi and carriage return", "\x1b[33mTo sign in, use a web browser to open the page https://microsoft.com/devicelogin and enter the code F7K2Q9XM to authenticate.\x1b[0m\r", "https://microsoft.com/devicelogin", "F7K2Q9XM", true},
		{"lowercase prose", "Please enter the code below to continue", "", "", false},
		{"lowercase after the phrase", "enter the code below", "", "", false},
		{"too short", "enter the code ZZ99", "", "", false},
		{"code shaped word without context", "Retrieving tenants and subscriptions ABCDEFGH for the selection", "", "", false},
		{"extra whitespace and prefix", "  WARNING:   open https://login.microsoft.com/device   and  enter   the  code   AB12CD34 ", "https://login.microsoft.com/device", "AB12CD34", true},
		{"fake az line", "fake-az: To sign in, use a web browser to open https://microsoft.com/devicelogin and enter the code FAKECODE", "https://microsoft.com/devicelogin", "FAKECODE", true},
		{"url without code", "To sign in, use a web browser to open the page https://microsoft.com/devicelogin", "", "", false},
		{"unrelated line", "Retrieving tenants and subscriptions for the selection...", "", "", false},
		{"code without url uses the default page", "enter the code ZZ99ZZ99", "https://microsoft.com/devicelogin", "ZZ99ZZ99", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, code, ok := azure.ParseDeviceCode(tc.line)
			if ok != tc.ok || url != tc.url || code != tc.code {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)", url, code, ok, tc.url, tc.code, tc.ok)
			}
		})
	}
}

func TestAzureLoginEmitsDeviceCodeEvent(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	var events []provider.LoginEvent
	if _, err := h.provider.Login(context.Background(), acct, provider.LoginIO{
		Progress: func(e provider.LoginEvent) { events = append(events, e) },
	}); err != nil {
		t.Fatal(err)
	}
	var codes []provider.LoginEvent
	for _, e := range events {
		if e.Kind == provider.LoginDeviceCode {
			codes = append(codes, e)
		}
	}
	if len(codes) != 1 || codes[0].URL != "https://microsoft.com/devicelogin" || codes[0].Code != "FAKECODE" {
		t.Fatalf("device code events %+v", codes)
	}
}

func TestAzureLoginStreamsOutputLines(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	var events []provider.LoginEvent
	if _, err := h.provider.Login(context.Background(), acct, provider.LoginIO{
		Progress: func(e provider.LoginEvent) { events = append(events, e) },
	}); err != nil {
		t.Fatal(err)
	}
	var output []string
	var steps []string
	for _, e := range events {
		switch e.Kind {
		case provider.LoginOutput:
			output = append(output, e.Output)
		case provider.LoginStepPending, provider.LoginStepStarted, provider.LoginStepDone, provider.LoginStepFailed:
			steps = append(steps, string(e.Kind)+" "+e.Step)
		}
	}
	joined := strings.Join(output, "\n")
	if !strings.Contains(joined, "enter the code FAKECODE") || !strings.Contains(joined, "fake-az: waiting") {
		t.Errorf("output lines %q", output)
	}
	wantSteps := []string{
		"step-pending isolate config",
		"step-pending az login --use-device-code",
		"step-pending az account set --subscription " + subscription,
		"step-pending verify az account get-access-token",
		"step-started isolate config", "step-done isolate config",
		"step-started az login --use-device-code", "step-done az login --use-device-code",
		"step-started az account set --subscription " + subscription, "step-done az account set --subscription " + subscription,
		"step-started verify az account get-access-token", "step-done verify az account get-access-token",
	}
	if strings.Join(steps, "|") != strings.Join(wantSteps, "|") {
		t.Errorf("steps\n%s\nwant\n%s", strings.Join(steps, "\n"), strings.Join(wantSteps, "\n"))
	}
}

func TestAzureLoginFailureMarksStepFailed(t *testing.T) {
	h := newHarness(t, "FAKE_AZ_LOGIN_FAIL=1")
	acct := h.created(t, "work")
	var failed []string
	_, err := h.provider.Login(context.Background(), acct, provider.LoginIO{
		Progress: func(e provider.LoginEvent) {
			if e.Kind == provider.LoginStepFailed {
				failed = append(failed, e.Step)
			}
		},
	})
	if err == nil || len(failed) != 1 || failed[0] != "az login --use-device-code" {
		t.Fatalf("err %v failed %v", err, failed)
	}
}

func TestAzureLoginCLIModeStillWritesToStdout(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	var stdout, stderr bytes.Buffer
	_, err := h.provider.Login(context.Background(), acct, provider.LoginIO{
		Stdout: &stdout, Stderr: &stderr,
		Progress: func(provider.LoginEvent) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "enter the code FAKECODE") {
		t.Errorf("stdout %q lacks the device code message", stdout.String())
	}
	if !strings.Contains(stderr.String(), "fake-az: waiting") {
		t.Errorf("stderr %q lacks the stderr line", stderr.String())
	}
}

func TestDeviceCodeScannerIgnoresProseAndFindsCodeOnNextLine(t *testing.T) {
	scanner := azure.NewDeviceCodeScanner()
	for _, line := range []string{"Please enter the code below to continue", "open https://microsoft.com/devicelogin", "and enter the code"} {
		if _, _, ok := scanner.Scan(line); ok {
			t.Fatalf("line %q matched", line)
		}
	}
	url, code, ok := scanner.Scan("  F7K2Q9XM ")
	if !ok || code != "F7K2Q9XM" || url != "https://microsoft.com/devicelogin" {
		t.Errorf("got (%q, %q, %v)", url, code, ok)
	}
}

func TestAzureLoginStrayLineDoesNotHideTheCode(t *testing.T) {
	h := newHarness(t, "FAKE_AZ_LOGIN_PROSE=1")
	acct := h.created(t, "work")
	var codes []string
	if _, err := h.provider.Login(context.Background(), acct, provider.LoginIO{
		Progress: func(e provider.LoginEvent) {
			if e.Kind == provider.LoginDeviceCode {
				codes = append(codes, e.Code)
			}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if len(codes) != 1 || codes[0] != "FAKECODE" {
		t.Errorf("codes %v, want [FAKECODE]", codes)
	}
}

func TestDeleteAzureKeepsDirWhenUnchecked(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	dir := acct.Settings["dir"]
	result, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir must stay: %v", err)
	}
	if len(result.Removed) != 0 {
		t.Errorf("removed %v", result.Removed)
	}
	if len(h.calls(t)) != 0 {
		t.Error("no az call expected without Logout")
	}
}

func TestDeleteAzureLogoutFailureIsWarning(t *testing.T) {
	h := newHarness(t, "FAKE_AZ_LOGOUT_FAIL=1")
	acct := h.created(t, "work")
	dir := acct.Settings["dir"]
	result, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{Logout: true, RemoveDir: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("dir should still be removed after a logout failure")
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != provider.WarnLogoutFailed {
		t.Errorf("warnings %+v", result.Warnings)
	}
}

func TestAzureDeleteItems(t *testing.T) {
	h := newHarness(t)
	acct := h.created(t, "work")
	if err := os.WriteFile(filepath.Join(acct.Settings["dir"], "msal_token_cache.json"), make([]byte, 2048), 0o600); err != nil {
		t.Fatal(err)
	}
	items := h.provider.DeleteItems(acct)
	if len(items) != 2 || items[0].Key != "remove_dir" || !items[0].Default || items[1].Key != "logout" || items[1].Default {
		t.Fatalf("items %+v", items)
	}
	if !strings.Contains(items[0].Label, acct.Settings["dir"]) || !strings.Contains(items[0].Detail, "2.0 KB") {
		t.Errorf("remove_dir item %+v", items[0])
	}
	if !strings.Contains(h.provider.Kept(acct), "Azure subscription") {
		t.Errorf("kept %q", h.provider.Kept(acct))
	}
}

func TestAzureDeleteLogoutTimeoutIsWarning(t *testing.T) {
	defer azure.SetLogoutTimeout(300 * time.Millisecond)()
	h := newHarness(t, "FAKE_AZ_LOGOUT_HANG=1")
	acct := h.created(t, "work")
	dir := acct.Settings["dir"]
	start := time.Now()
	result, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{Logout: true, RemoveDir: true})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Delete took %s with a 300ms logout bound", elapsed)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("deletion must continue after a logout timeout")
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != provider.WarnLogoutFailed || !strings.Contains(result.Warnings[0].Message, "work") {
		t.Errorf("warnings %+v", result.Warnings)
	}
}
