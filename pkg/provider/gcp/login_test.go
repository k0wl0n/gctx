package gcp_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/gcp"
)

const missingCodeStderr = "ERROR: (gcloud.auth.login) There was a problem with web authentication.\n" +
	"(missing_code) Missing code parameter in response.\n"

// scriptedRunner fails interactive steps per their joined args, in order.
type scriptedRunner struct {
	calls    []string
	failures map[string][]string // args -> stderr text of each failing attempt
	cancel   context.CancelFunc  // when set, cancels on the first interactive call
}

func (s *scriptedRunner) Query(_ context.Context, _ []string, args ...string) (string, error) {
	s.calls = append(s.calls, strings.Join(args, " "))
	return "", nil
}

func (s *scriptedRunner) Interactive(_ context.Context, stdio provider.Stdio, _ []string, args ...string) error {
	line := strings.Join(args, " ")
	s.calls = append(s.calls, line)
	if s.cancel != nil {
		s.cancel()
		return errors.New("signal: interrupt")
	}
	if queue := s.failures[line]; len(queue) > 0 {
		s.failures[line] = queue[1:]
		if stdio.Stderr != nil {
			io.WriteString(stdio.Stderr, queue[0])
		}
		return errors.New("exit status 1")
	}
	return nil
}

func (s *scriptedRunner) WithEnv([]string) provider.Runner { return s }

func (s *scriptedRunner) count(line string) int {
	n := 0
	for _, c := range s.calls {
		if c == line {
			n++
		}
	}
	return n
}

func loginProvider(t *testing.T, runner provider.Runner) *gcp.Provider {
	t.Helper()
	gcloudDir := t.TempDir()
	defaultADC := filepath.Join(gcloudDir, "application_default_credentials.json")
	writeFile(t, defaultADC, `{"type":"authorized_user"}`)
	return gcp.New(gcp.Options{
		GcloudDir:  gcloudDir,
		ADCDir:     t.TempDir(),
		DefaultADC: defaultADC,
		Client:     &gcloud.Client{Bin: "gcloud"},
		Runner:     runner,
		Callback:   freePortPreflight(t),
	})
}

var loginAcct = config.Account{Name: "work", ConfigName: "gctx-work", ProjectID: "p1"}

const (
	authLine = "auth login"
	adcLine  = "auth application-default login"
)

func TestLoginClassifiesCallbackFailure(t *testing.T) {
	runner := &scriptedRunner{failures: map[string][]string{authLine: {missingCodeStderr}}}
	_, err := loginProvider(t, runner).Login(context.Background(), loginAcct, provider.LoginIO{})
	if !errors.Is(err, provider.ErrBrowserCallback) {
		t.Fatalf("err %v is not ErrBrowserCallback", err)
	}
	if err.Error() != "auth login failed: exit status 1" {
		t.Errorf("message changed: %q", err.Error())
	}
}

func TestLoginOtherFailureIsNotCallback(t *testing.T) {
	runner := &scriptedRunner{failures: map[string][]string{adcLine: {"ERROR: network unreachable\n"}}}
	retried := false
	lio := provider.LoginIO{RetryStep: func(string, error) bool { retried = true; return true }}
	_, err := loginProvider(t, runner).Login(context.Background(), loginAcct, lio)
	if err == nil || errors.Is(err, provider.ErrBrowserCallback) {
		t.Fatalf("err %v must be a plain failure", err)
	}
	if retried {
		t.Error("a non-callback failure must not offer a retry")
	}
	if err.Error() != "ADC auth failed: exit status 1" {
		t.Errorf("message %q", err.Error())
	}
}

func TestLoginCancelIsNotCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &scriptedRunner{cancel: cancel}
	retried := false
	lio := provider.LoginIO{RetryStep: func(string, error) bool { retried = true; return true }}
	_, err := loginProvider(t, runner).Login(ctx, loginAcct, lio)
	if err == nil || errors.Is(err, provider.ErrBrowserCallback) || retried {
		t.Fatalf("err %v retried %v", err, retried)
	}
}

func TestLoginRetriesFirstStepWithoutReactivating(t *testing.T) {
	runner := &scriptedRunner{failures: map[string][]string{authLine: {missingCodeStderr}}}
	var steps []string
	lio := provider.LoginIO{RetryStep: func(step string, err error) bool {
		steps = append(steps, step)
		return true
	}}
	result, err := loginProvider(t, runner).Login(context.Background(), loginAcct, lio)
	if err != nil {
		t.Fatalf("login after retry: %v", err)
	}
	if fmt.Sprint(steps) != "["+provider.LoginStepAuth+"]" {
		t.Errorf("retry asked for %v", steps)
	}
	if runner.count(authLine) != 2 || runner.count(adcLine) != 1 {
		t.Errorf("calls %v", runner.calls)
	}
	if runner.count("config configurations activate gctx-work") != 1 {
		t.Errorf("activation repeated: %v", runner.calls)
	}
	if !result.GcloudLoggedIn || !result.ADCSaved {
		t.Errorf("result %+v", result)
	}
}

func TestLoginDeclinedRetryReturnsWrappedError(t *testing.T) {
	runner := &scriptedRunner{failures: map[string][]string{authLine: {missingCodeStderr}}}
	lio := provider.LoginIO{RetryStep: func(string, error) bool { return false }}
	_, err := loginProvider(t, runner).Login(context.Background(), loginAcct, lio)
	if !errors.Is(err, provider.ErrBrowserCallback) || !strings.HasPrefix(err.Error(), "auth login failed:") {
		t.Fatalf("err %v", err)
	}
	if runner.count(authLine) != 1 {
		t.Errorf("calls %v", runner.calls)
	}
}

func TestLoginADCRetryKeepsFirstStep(t *testing.T) {
	runner := &scriptedRunner{failures: map[string][]string{adcLine: {missingCodeStderr}}}
	var steps []string
	lio := provider.LoginIO{RetryStep: func(step string, err error) bool {
		steps = append(steps, step)
		return true
	}}
	if _, err := loginProvider(t, runner).Login(context.Background(), loginAcct, lio); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(steps) != "["+provider.LoginStepADC+"]" || runner.count(authLine) != 1 || runner.count(adcLine) != 2 {
		t.Errorf("steps %v calls %v", steps, runner.calls)
	}
}

func TestLoginNeverPassesNoBrowser(t *testing.T) {
	runner := &scriptedRunner{}
	if _, err := loginProvider(t, runner).Login(context.Background(), loginAcct, provider.LoginIO{NoBrowser: true}); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.calls {
		if strings.Contains(call, "--no-browser") {
			t.Errorf("gcloud got --no-browser: %v", runner.calls)
		}
	}
}
