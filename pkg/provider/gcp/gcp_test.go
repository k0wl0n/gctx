package gcp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/gcp"
)

var _ provider.Provider = (*gcp.Provider)(nil)

type fakeRunner struct {
	outputs map[string]string
	calls   []string
}

func (f *fakeRunner) Query(_ context.Context, _ []string, args ...string) (string, error) {
	line := strings.Join(args, " ")
	f.calls = append(f.calls, line)
	return f.outputs[line], nil
}

func (f *fakeRunner) Interactive(_ context.Context, _ provider.Stdio, _ []string, args ...string) error {
	f.calls = append(f.calls, strings.Join(args, " "))
	return nil
}

func (f *fakeRunner) WithEnv([]string) provider.Runner { return f }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGCPLocalMatchesLegacyStatus(t *testing.T) {
	root := t.TempDir()
	gcloudDir := filepath.Join(root, "gcloud")
	adcDir := filepath.Join(root, "adc")
	writeFile(t, filepath.Join(gcloudDir, "configurations", "config_work-config"),
		"[core]\naccount = work@example.com\nproject = work-project\n")
	adcPath := filepath.Join(adcDir, "work_adc.json")
	writeFile(t, adcPath, `{"type":"authorized_user"}`)

	p := gcp.New(gcp.Options{
		GcloudDir:  gcloudDir,
		ADCDir:     adcDir,
		DefaultADC: filepath.Join(gcloudDir, "application_default_credentials.json"),
		Client:     &gcloud.Client{},
	})
	acct := config.Account{Name: "work", ConfigName: "work-config", ProjectID: "work-project", ADCPath: adcPath}

	local := p.Local(acct)
	if local.Status != provider.HealthReady || len(local.Findings) != 0 {
		t.Fatalf("status %s findings %+v, want READY", local.Status, local.Findings)
	}
	if local.Identity != "work@example.com" {
		t.Errorf("identity %q", local.Identity)
	}

	acct.ProjectID = "other-project"
	if got := p.Local(acct).Status; got != provider.HealthProjectMismatch {
		t.Errorf("mismatched project: status %s", got)
	}
}

func TestGCPCheckAuthTreatsCredentialedAsAuthed(t *testing.T) {
	runner := &fakeRunner{outputs: map[string]string{
		"auth list --format=json": `[{"account":"Active@example.com","status":"ACTIVE"},` +
			`{"account":"stored@example.com","status":"CREDENTIALED"},` +
			`{"account":"other@example.com","status":""}]`,
	}}
	p := gcp.New(gcp.Options{
		GcloudDir: t.TempDir(),
		ADCDir:    t.TempDir(),
		Client:    &gcloud.Client{Bin: "gcloud"},
		Runner:    runner,
	})

	report := p.CheckAuth(context.Background(), nil)
	if len(report.Warnings) != 0 {
		t.Fatalf("warnings %+v", report.Warnings)
	}
	want := map[string]bool{"active@example.com": true, "stored@example.com": true}
	if len(report.Results) != len(want) {
		t.Fatalf("credentialed %v, want %v", report.Results, want)
	}
	for account := range want {
		if !report.Results[account].Credentialed {
			t.Errorf("%s not credentialed: %v", account, report.Results)
		}
	}
}

func TestGCPCheckAuthWithoutGcloudWarns(t *testing.T) {
	p := gcp.New(gcp.Options{GcloudDir: t.TempDir(), ADCDir: t.TempDir(), Client: &gcloud.Client{}})
	report := p.CheckAuth(context.Background(), nil)
	if len(report.Warnings) != 1 || report.Warnings[0].Code != provider.WarnGcloudNotFound {
		t.Fatalf("warnings %+v", report.Warnings)
	}
}

func TestGCPEnvIsolatesAccount(t *testing.T) {
	p := gcp.New(gcp.Options{GcloudDir: t.TempDir(), ADCDir: t.TempDir(), Client: &gcloud.Client{}})
	env, err := p.Env(config.Account{Name: "work", ConfigName: "work-config", ADCPath: "/adc/work.json"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"CLOUDSDK_ACTIVE_CONFIG_NAME=work-config", "GOOGLE_APPLICATION_CREDENTIALS=/adc/work.json"}
	if strings.Join(env, "\n") != strings.Join(want, "\n") {
		t.Errorf("env %q, want %q", env, want)
	}
}
