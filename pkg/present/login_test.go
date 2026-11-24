package present_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestRetryPromptDefaultsToYes(t *testing.T) {
	var out strings.Builder
	retry := present.RetryPrompt(strings.NewReader("\n"), &out)
	if !retry(provider.LoginStepAuth, errors.New("x")) {
		t.Error("empty answer must retry")
	}
	for _, want := range []string{"Retry auth login only? [Y/n] ", "localhost:8085", "Close old Google sign-in tabs"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("prompt lacks %q: %q", want, out.String())
		}
	}
}

func TestRetryPromptNoDeclinesAndNamesStep(t *testing.T) {
	var out strings.Builder
	retry := present.RetryPrompt(strings.NewReader("n\ny\n"), &out)
	if retry(provider.LoginStepADC, errors.New("x")) {
		t.Error("n must decline")
	}
	if !strings.Contains(out.String(), "Retry application-default login only? [Y/n]") {
		t.Errorf("prompt %q", out.String())
	}
	if !retry(provider.LoginStepADC, errors.New("x")) {
		t.Error("second answer y must retry: the reader must not swallow later input")
	}
}

func TestLoginDoneCallbackGuidance(t *testing.T) {
	var out, errw strings.Builder
	err := fmt.Errorf("auth login failed: %w", errors.Join(errors.New("exit status 1"), provider.ErrBrowserCallback))
	present.LoginDone(&out, &errw, manager.LoginResult{Account: "work"}, err)
	for _, want := range []string{"localhost:8085", "only one sign-in tab", "GCTX_DEBUG=1", "gctx login work"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("guidance lacks %q: %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "You cancelled") {
		t.Errorf("generic text shown for a callback failure: %q", out.String())
	}
}

func TestLoginDoneGenericADCFailureUnchanged(t *testing.T) {
	var out, errw strings.Builder
	err := &gcloud.StepError{Step: manager.StepADCLogin, Err: errors.New("cancelled")}
	present.LoginDone(&out, &errw, manager.LoginResult{Account: "work"}, err)
	if !strings.Contains(out.String(), "You cancelled the authentication (Ctrl+C)") || strings.Contains(out.String(), "8085") {
		t.Errorf("generic text %q", out.String())
	}
}
