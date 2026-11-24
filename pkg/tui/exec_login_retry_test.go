package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// callbackFailOnce emulates a provider whose first step fails once with a
// browser callback error and asks RetryStep before re-running only it.
func callbackFailOnce(attempts *[]string) func(context.Context, string, manager.LoginIO) (manager.LoginResult, error) {
	return func(_ context.Context, name string, lio manager.LoginIO) (manager.LoginResult, error) {
		if lio.NoBrowser {
			return manager.LoginResult{}, errors.New("--no-browser must not be requested")
		}
		*attempts = append(*attempts, provider.LoginStepAuth)
		err := fmt.Errorf("auth login failed: %w", errors.Join(errors.New("exit status 1"), provider.ErrBrowserCallback))
		if lio.RetryStep == nil || !lio.RetryStep(provider.LoginStepAuth, err) {
			return manager.LoginResult{Account: name}, err
		}
		*attempts = append(*attempts, provider.LoginStepAuth, provider.LoginStepADC)
		return manager.LoginResult{Account: name, GcloudLoggedIn: true, ADCSaved: true}, nil
	}
}

func TestLoginExecOffersStepRetryOnCallbackFailure(t *testing.T) {
	svc := newFakeService(sampleOverview("sandbox", "sandbox"))
	var attempts []string
	svc.loginFn = callbackFailOnce(&attempts)
	var out strings.Builder
	e := &loginExec{stdin: strings.NewReader("y\n"), stdout: &out, stderr: &out, svc: svc, name: "sandbox", configName: "gctx-sandbox"}

	err := e.Run()
	var le *loginError
	if !errors.As(err, &le) || le.err != nil {
		t.Fatalf("login after retry: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Retry auth login only? [Y/n]") {
		t.Errorf("no retry prompt: %s", out.String())
	}
	if fmt.Sprint(attempts) != "[auth login auth login application-default login]" {
		t.Errorf("attempts %v", attempts)
	}
	if strings.Contains(out.String(), "retrying without browser") {
		t.Errorf("old no-browser retry still runs: %s", out.String())
	}
}

func TestLoginExecDeclinedRetryFails(t *testing.T) {
	svc := newFakeService(sampleOverview("sandbox", "sandbox"))
	var attempts []string
	svc.loginFn = callbackFailOnce(&attempts)
	var out strings.Builder
	e := &loginExec{stdin: strings.NewReader("n\n\n"), stdout: &out, stderr: &out, svc: svc, name: "sandbox", configName: "gctx-sandbox"}

	var le *loginError
	if err := e.Run(); !errors.As(err, &le) || !errors.Is(le.err, provider.ErrBrowserCallback) {
		t.Fatalf("err %v", err)
	}
	if len(attempts) != 1 || !strings.Contains(out.String(), "localhost:8085") {
		t.Errorf("attempts %v out %s", attempts, out.String())
	}
}

func TestBrowserFailureNoBrowserRetryRemoved(t *testing.T) {
	src, err := os.ReadFile("exec.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "isBrowserAuthFailure") || strings.Contains(string(src), "NoBrowser") {
		t.Error("exec.go still carries the --no-browser auto-retry")
	}
}
