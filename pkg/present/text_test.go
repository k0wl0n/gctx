package present_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestEnvQuotesSingleQuotes(t *testing.T) {
	var out bytes.Buffer
	present.Env(&out, []string{"A=plain", "B=it's", "C=a=b"})
	want := "export A='plain'\nexport B='it'\\''s'\nexport C='a=b'\n"
	if out.String() != want {
		t.Errorf("got %q want %q", out.String(), want)
	}
}

func TestSwitchPrintsApplyHint(t *testing.T) {
	var out, errw bytes.Buffer
	present.Switch(&out, &errw, manager.SwitchResult{Account: "prod", ProjectID: "p", ApplyHint: "run: eval $(gctx env prod)"})
	if !strings.HasSuffix(out.String(), "Switched to account: prod (p)\nrun: eval $(gctx env prod)\n") {
		t.Errorf("got %q", out.String())
	}
}

func accountsOf(ids ...provider.ID) manager.Overview {
	var overview manager.Overview
	for _, id := range ids {
		overview.Accounts = append(overview.Accounts, manager.AccountStatus{Name: string(id) + "-acct", Provider: id, ProjectID: "t"})
	}
	return overview
}

func TestListProviderColumn(t *testing.T) {
	var gcpOnly, mixed bytes.Buffer
	present.List(&gcpOnly, accountsOf(provider.GCP), present.ListOptions{})
	if strings.Contains(gcpOnly.String(), "PROVIDER") {
		t.Errorf("gcp-only: %q", gcpOnly.String())
	}
	present.List(&mixed, accountsOf(provider.GCP, provider.AWS), present.ListOptions{})
	if !strings.Contains(mixed.String(), "PROVIDER") {
		t.Errorf("mixed: %q", mixed.String())
	}
	var forced bytes.Buffer
	present.List(&forced, accountsOf(provider.GCP), present.ListOptions{Filter: provider.GCP})
	if !strings.Contains(forced.String(), "PROVIDER") {
		t.Errorf("filtered: %q", forced.String())
	}
}

func TestInfoShowsProviderForNonGCPOnly(t *testing.T) {
	var gcpOut, awsOut bytes.Buffer
	present.Info(&gcpOut, manager.AccountStatus{Name: "a", Provider: provider.GCP, ProjectID: "p"})
	if strings.Contains(gcpOut.String(), "Provider:") {
		t.Errorf("gcp: %q", gcpOut.String())
	}
	present.Info(&awsOut, manager.AccountStatus{Name: "a", Provider: provider.AWS, TargetLabel: "PROFILE", ProjectID: "dev"})
	if !strings.Contains(awsOut.String(), "Provider:") || !strings.Contains(awsOut.String(), "aws") || !strings.Contains(awsOut.String(), "PROFILE") {
		t.Errorf("aws: %q", awsOut.String())
	}
}

func TestLoginDonePrintsProfileIncompleteWarning(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := manager.LoginResult{
		Account: "work",
		Warnings: []provider.Warning{{
			Code: provider.WarnProfileIncomplete, Account: "work",
			Message: "profile work lacks sso_account_id and sso_role_name; credential checks fail until set:\n" +
				"aws configure set profile.work.sso_account_id ACCOUNT_ID\naws configure set profile.work.sso_role_name ROLE_NAME",
		}},
	}
	present.LoginDone(&stdout, &stderr, result, nil)
	for _, want := range []string{"sso_account_id", "sso_role_name", "aws configure set profile.work.sso_account_id ACCOUNT_ID"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr %q lacks %q", stderr.String(), want)
		}
	}
}

func TestLoginDoneOmitsADCLinesWithoutADC(t *testing.T) {
	var stdout, stderr bytes.Buffer
	present.LoginDone(&stdout, &stderr, manager.LoginResult{Account: "work"}, nil)
	if strings.Contains(stdout.String(), "ADC") || strings.Contains(stdout.String(), "Saved to") {
		t.Errorf("stdout %q mentions ADC", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Account 'work' is ready to use!") {
		t.Errorf("stdout %q", stdout.String())
	}

	stdout.Reset()
	present.LoginDone(&stdout, &stderr, manager.LoginResult{Account: "g", ADCSaved: true, ADCPath: "/p/adc.json"}, nil)
	if !strings.Contains(stdout.String(), "ADC credentials auto-saved for: g\nSaved to: /p/adc.json\n") {
		t.Errorf("stdout %q", stdout.String())
	}
}

func TestLoginDoneDoesNotRepeatSetProjectFailed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result := manager.LoginResult{
		Account:  "work",
		Warnings: []provider.Warning{{Code: provider.WarnSetProjectFailed, Account: "work", Message: "could not set project"}},
	}
	present.LoginDone(&stdout, &stderr, result, nil)
	if stderr.Len() != 0 {
		t.Errorf("stderr %q repeats the set-project warning", stderr.String())
	}
}
