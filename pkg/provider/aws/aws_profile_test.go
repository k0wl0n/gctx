package aws_test

import (
	"context"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestAWSProfileMustNotLookLikeAFlag(t *testing.T) {
	h := newHarness(t, ssoConfig)
	for _, name := range []string{"--debug", "-x", ".hidden"} {
		if err := h.provider.ValidateTarget(name); err == nil {
			t.Errorf("ValidateTarget(%q) accepted a flag-like profile", name)
		}
	}
	for _, name := range []string{"work", "_x", "9a.b-c"} {
		if err := h.provider.ValidateTarget(name); err != nil {
			t.Errorf("ValidateTarget(%q) = %v", name, err)
		}
	}
}

// TestAWSRefusesFlagProfileFromEditedConfig covers a hand-edited config whose
// target skipped create-time validation: no aws command may run with it.
func TestAWSRefusesFlagProfileFromEditedConfig(t *testing.T) {
	h := newHarness(t, ssoConfig+"\n[profile --debug]\nsso_start_url = https://x\nsso_region = eu-west-1\n")
	bad := account("bad", "--debug")
	report := h.provider.CheckAuth(context.Background(), []config.Account{bad})
	if report.Results["bad"].Credentialed {
		t.Errorf("check auth %+v", report.Results["bad"])
	}
	if _, err := h.provider.Login(context.Background(), bad, provider.LoginIO{}); err == nil {
		t.Error("login accepted --debug")
	}
	result, err := h.provider.Delete(context.Background(), bad, provider.DeleteOptions{Logout: true})
	if err != nil || len(result.Warnings) != 1 {
		t.Errorf("delete %+v err %v, want one warning", result, err)
	}
	if calls := h.calls(t); len(calls) != 0 {
		t.Errorf("aws ran with a flag-like profile: %v", calls)
	}
}

func TestAWSPassesProfileAsOneArgument(t *testing.T) {
	h := newHarness(t, ssoConfig)
	acct := account("work", "work")
	h.provider.CheckAuth(context.Background(), []config.Account{acct})
	if _, err := h.provider.Login(context.Background(), acct, provider.LoginIO{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.provider.Delete(context.Background(), acct, provider.DeleteOptions{Logout: true}); err != nil {
		t.Fatal(err)
	}
	for _, call := range h.calls(t) {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "--profile ") || !strings.Contains(joined, "--profile=work") {
			t.Errorf("call %q does not pass --profile=work", joined)
		}
	}
}

func TestAWSLogoutItemSaysItSignsOutEverySession(t *testing.T) {
	items := newHarness(t, ssoConfig).provider.DeleteItems(account("work", "work"))
	if len(items) != 1 || items[0].Default || !strings.Contains(items[0].Detail, "every AWS SSO session on this machine") {
		t.Fatalf("items %+v", items)
	}
}
