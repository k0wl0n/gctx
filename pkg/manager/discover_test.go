package manager_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// Values that must never reach config.json, the status cache or any output.
var fakeSecrets = []string{"FAKE-REFRESH-TOKEN", "FAKE-ACCESS-TOKEN", "FAKESECRETKEY", "FAKE-SESSION-TOKEN", "FAKE-GCLOUD-TOKEN-FILE"}

type discoveryFixture struct {
	home    string
	paths   manager.Paths
	manager *manager.Manager
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newDiscoveryFixture builds a home directory with gcloud, aws and azure
// files, some holding secret-looking fields, and a manager over it.
func newDiscoveryFixture(t *testing.T, lookPath func(string) (string, error)) discoveryFixture {
	t.Helper()
	home := t.TempDir()
	env := map[string]string{}
	paths := manager.ResolvePaths(func(k string) string { return env[k] }, "linux", home)
	paths.CacheDir = filepath.Join(home, "cache")

	gcloudConfigs := filepath.Join(paths.GcloudDir, "configurations")
	writeFixtureFile(t, filepath.Join(gcloudConfigs, "config_default"),
		"[core]\naccount = me@example.com\nproject = default-project\n")
	writeFixtureFile(t, filepath.Join(gcloudConfigs, "config_Work_Prod"),
		"[core]\naccount = ops@example.com\nproject = work-prod-1\n[auth]\naccess_token_file = /FAKE-GCLOUD-TOKEN-FILE\n")
	writeFixtureFile(t, filepath.Join(gcloudConfigs, "config_9lives"), "[core]\nproject = nine-lives\n")
	writeFixtureFile(t, filepath.Join(paths.GcloudDir, "application_default_credentials.json"), `{"refresh_token":"FAKE-REFRESH-TOKEN"}`)

	writeFixtureFile(t, filepath.Join(home, ".aws", "config"), `[default]
region = eu-west-1
aws_session_token = FAKE-SESSION-TOKEN

[profile prod-sso]
sso_session = corp
sso_account_id = 123456789012
sso_role_name = PlatformDev
region = eu-west-1

[profile legacy]
region = us-east-1
aws_secret_access_key = FAKESECRETKEY

[profile weird name]
region = us-east-2

[sso-session corp]
sso_start_url = https://corp.awsapps.com/start
sso_region = eu-west-1
`)
	writeFixtureFile(t, filepath.Join(home, ".aws", "credentials"), "[default]\naws_secret_access_key = FAKESECRETKEY\n")

	writeFixtureFile(t, filepath.Join(home, ".azure", "azureProfile.json"), "\ufeff"+`{"installationId":"x","subscriptions":[
 {"id":"8c1f0a3e-0000-4000-8000-00000000e2a9","name":"Corp Subscription","tenantId":"11111111-0000-4000-8000-000000000001","isDefault":true,"user":{"name":"dev@corp.example"},"accessToken":"FAKE-ACCESS-TOKEN"},
 {"id":"2d7b6c10-0000-4000-8000-00000000a1b2","name":"Lab","tenantId":"11111111-0000-4000-8000-000000000001"}]}`)
	writeFixtureFile(t, filepath.Join(home, ".azure", "msal_token_cache.json"), `{"AccessToken":{"x":{"secret":"FAKE-ACCESS-TOKEN"}}}`)

	if lookPath == nil {
		lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	}
	m, err := manager.NewWithOptions(manager.Options{
		Paths:    paths,
		Home:     home,
		Getenv:   func(k string) string { return env[k] },
		LookPath: lookPath,
		Now:      func() time.Time { return time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return discoveryFixture{home: home, paths: paths, manager: m}
}

func itemsOf(result manager.DiscoveryResult, id provider.ID) map[string]manager.DiscoveredAccount {
	out := map[string]manager.DiscoveredAccount{}
	for _, item := range result.Items {
		if item.Provider == id {
			out[item.Name] = item
		}
	}
	return out
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out[path] = fmt.Sprintf("%v %d %d", info.IsDir(), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return out
}

func TestDiscoverIsReadOnly(t *testing.T) {
	t.Setenv("PATH", "")
	fx := newDiscoveryFixture(t, nil)
	before := snapshotTree(t, fx.home)

	result, err := fx.manager.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if after := snapshotTree(t, fx.home); fmt.Sprint(before) != fmt.Sprint(after) {
		t.Errorf("discovery changed the home directory:\nbefore %v\nafter  %v", before, after)
	}

	gcp := itemsOf(result, provider.GCP)
	if len(gcp) != 3 || gcp["default"].Target != "default-project" || gcp["default"].Email != "me@example.com" || gcp["default"].ConfigName != "default" {
		t.Errorf("gcloud configurations: %+v", gcp)
	}
	if work := gcp["work-prod"]; work.ConfigName != "Work_Prod" || work.Target != "work-prod-1" {
		t.Errorf("gcloud name is the sanitized suffix and ConfigName the original: %+v", work)
	}

	aws := itemsOf(result, provider.AWS)
	if len(aws) != 4 {
		t.Fatalf("aws profiles: %+v", aws)
	}
	sso := aws["prod-sso"]
	if sso.Settings["auth"] != "profile" || sso.Settings["region"] != "eu-west-1" ||
		sso.Settings["sso_start_url"] != "https://corp.awsapps.com/start" || sso.Settings["sso_region"] != "eu-west-1" || sso.Note == "" {
		t.Errorf("sso profile: %+v", sso)
	}
	if def := aws["default"]; def.Target != "default" || def.Settings["region"] != "eu-west-1" {
		t.Errorf("default profile: %+v", def)
	}

	azure := itemsOf(result, provider.Azure)
	corp := azure["corp-subscription"]
	if len(azure) != 2 || corp.Target != "8c1f0a3e-0000-4000-8000-00000000e2a9" ||
		corp.Settings["dir"] != filepath.Join(fx.paths.GctxDir, "azure", "corp-subscription") {
		t.Errorf("azure subscriptions: %+v", azure)
	}
	if _, err := os.Stat(corp.Settings["dir"]); err == nil {
		t.Error("discovery created the isolated azure dir; first login does that")
	}

	counts := map[provider.ID]int{}
	for _, source := range result.Sources {
		counts[source.Provider] = source.Count
	}
	if counts[provider.GCP] != 3 || counts[provider.AWS] != 4 || counts[provider.Azure] != 2 {
		t.Errorf("source counts %v", counts)
	}
	for _, source := range result.Sources {
		if source.Noun == "" {
			t.Errorf("source %s lacks its noun", source.Provider)
		}
	}
	assertNoSecrets(t, "discovery result", fmt.Sprintf("%+v", result))
}

func assertNoSecrets(t *testing.T, what, text string) {
	t.Helper()
	for _, secret := range fakeSecrets {
		if strings.Contains(text, secret) {
			t.Errorf("%s contains %q", what, secret)
		}
	}
}

func TestImportCopiesNamesOnly(t *testing.T) {
	t.Setenv("PATH", "")
	fx := newDiscoveryFixture(t, nil)
	result, err := fx.manager.Discover()
	if err != nil {
		t.Fatal(err)
	}
	imported, err := fx.manager.ImportAccounts(result.Items)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Imported) == 0 {
		t.Fatalf("nothing imported: %+v", imported)
	}

	raw, err := os.ReadFile(fx.paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecrets(t, "config.json", string(raw))
	assertNoSecrets(t, "import result", fmt.Sprintf("%+v", imported))

	cfg, err := config.LoadFrom(fx.paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	prod := cfg.Accounts["prod-sso"]
	if prod == nil || prod.Provider != "aws" || prod.ProjectID != "prod-sso" || prod.Settings["auth"] != "profile" || prod.Settings["region"] != "eu-west-1" {
		t.Fatalf("aws account: %+v", prod)
	}
	for key := range prod.Settings {
		if !slicesContains([]string{"auth", "region", "sso_start_url", "sso_region", "sso_account_id", "sso_role_name"}, key) {
			t.Errorf("unexpected stored setting %q", key)
		}
	}
	corp := cfg.Accounts["corp-subscription"]
	if corp == nil || corp.Provider != "azure" || corp.ProjectID != "8c1f0a3e-0000-4000-8000-00000000e2a9" ||
		corp.Settings["dir"] != filepath.Join(fx.paths.GctxDir, "azure", "corp-subscription") {
		t.Fatalf("azure account: %+v", corp)
	}
	if _, err := os.Stat(corp.Settings["dir"]); err == nil {
		t.Error("import created the isolated azure dir")
	}
	if cfg.ActiveAccount != "" || len(cfg.ActiveBySlot) != 0 {
		t.Errorf("import activated an account: %q %v", cfg.ActiveAccount, cfg.ActiveBySlot)
	}
	if entries, _ := os.ReadDir(fx.paths.ADCDir); len(entries) != 0 {
		t.Errorf("import saved ADC files: %v", entries)
	}

	// The status cache is written only by credential checks; whatever it holds
	// must be free of secrets too.
	m := fx.manager
	if _, err := m.Overview(); err != nil {
		t.Fatal(err)
	}
	if cache, err := os.ReadFile(filepath.Join(fx.paths.CacheDir, "status.json")); err == nil {
		assertNoSecrets(t, "status cache", string(cache))
	}
}

func slicesContains(list []string, value string) bool { return slices.Contains(list, value) }

func TestImportSanitizesAndSkipsInvalidNames(t *testing.T) {
	t.Setenv("PATH", "")
	fx := newDiscoveryFixture(t, nil)
	existing := config.Account{Name: "taken", ProjectID: "some-project", ConfigName: "taken-config"}
	cfg, err := config.LoadFrom(fx.paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.AddAccount(&existing); err != nil {
		t.Fatal(err)
	}

	items := []manager.DiscoveredAccount{
		{Provider: provider.GCP, Name: "My_Project.EU", Target: "my-project-eu", ConfigName: "My_Project.EU"},
		{Provider: provider.GCP, Name: "9lives", Target: "nine-lives", ConfigName: "9lives"},
		{Provider: provider.GCP, Name: "taken", Target: "other-project", ConfigName: "taken"},
		{Provider: provider.AWS, Name: "My_Project.EU", Target: "prod", Settings: map[string]string{"auth": "profile"}},
		{Provider: provider.AWS, Name: "bad-target", Target: "has space", Settings: map[string]string{"auth": "profile"}},
		{Provider: provider.GCP, Name: "no-project", ConfigName: "no-project"},
		{Provider: "mystery", Name: "mystery-acct", Target: "x"},
	}
	result, err := fx.manager.ImportAccounts(items)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Imported, []string{"my-project-eu"}) {
		t.Errorf("imported %v, want only my-project-eu", result.Imported)
	}
	reasons := map[string]string{}
	for _, skipped := range result.Skipped {
		reasons[skipped.Name] = skipped.Reason
	}
	for name, want := range map[string]string{
		"9lives": "invalid", "taken": "already", "bad-target": "invalid", "no-project": "project", "mystery-acct": "provider",
	} {
		if !strings.Contains(strings.ToLower(reasons[name]), want) {
			t.Errorf("skip reason for %q = %q, want it to mention %q", name, reasons[name], want)
		}
	}
	if len(result.Skipped) != 6 {
		t.Errorf("skipped %d, want 6 (the second My_Project.EU collides across providers): %+v", len(result.Skipped), result.Skipped)
	}

	cfg, err = config.LoadFrom(fx.paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Accounts["my-project-eu"] == nil || cfg.Accounts["taken"].ProjectID != "some-project" {
		t.Errorf("accounts after import: %v", cfg.Accounts)
	}
	if len(cfg.Accounts) != 2 {
		t.Errorf("config holds %d accounts, want 2", len(cfg.Accounts))
	}
}

func TestImportGCPReusesExistingConfigName(t *testing.T) {
	t.Setenv("PATH", "")
	fx := newDiscoveryFixture(t, nil)
	result, err := fx.manager.Discover()
	if err != nil {
		t.Fatal(err)
	}
	var gcpItems []manager.DiscoveredAccount
	for _, item := range result.Items {
		if item.Provider == provider.GCP {
			gcpItems = append(gcpItems, item)
		}
	}
	if _, err := fx.manager.ImportAccounts(gcpItems); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFrom(fx.paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	work := cfg.Accounts["work-prod"]
	if work == nil || work.ConfigName != "Work_Prod" || work.ProjectID != "work-prod-1" || work.Email != "ops@example.com" || work.Provider != "" {
		t.Fatalf("gcp account: %+v", work)
	}
	if def := cfg.Accounts["default"]; def == nil || def.ConfigName != "default" {
		t.Errorf("default: %+v", def)
	}
	entries, err := os.ReadDir(filepath.Join(fx.paths.GcloudDir, "configurations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("gcloud configurations changed: %v", entries)
	}

	status, err := fx.manager.Info("work-prod")
	if err != nil {
		t.Fatal(err)
	}
	if !status.GcloudConfigExists || status.GcloudAccount != "ops@example.com" {
		t.Errorf("imported account does not see the existing configuration: %+v", status)
	}
	if status.Status != manager.HealthADCMissing {
		t.Errorf("status %s, want ADC MISSING until logged in", status.Status)
	}
}

func fakeCLI(t *testing.T, name, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIVersionsRunsVersionPerFoundCLI(t *testing.T) {
	gcloud := fakeCLI(t, "gcloud", `echo "  Google Cloud SDK 540.0.0  "; echo "bq 2.1"`)
	aws := fakeCLI(t, "aws", `echo "aws-cli/2.27.4 Python/3.13"`)
	broken := fakeCLI(t, "az", `echo boom >&2; exit 3`)
	bins := map[string]string{"gcloud": gcloud, "aws": aws, "az": broken}
	fx := newDiscoveryFixture(t, func(name string) (string, error) {
		if bin, ok := bins[name]; ok {
			return bin, nil
		}
		return "", exec.ErrNotFound
	})

	versions := fx.manager.CLIVersions(context.Background())
	if versions[provider.GCP] != "Google Cloud SDK 540.0.0" {
		t.Errorf("gcloud: %q, want the trimmed first line", versions[provider.GCP])
	}
	if versions[provider.AWS] != "aws-cli/2.27.4 Python/3.13" {
		t.Errorf("aws: %q", versions[provider.AWS])
	}
	if got, ok := versions[provider.Azure]; ok && got != "" {
		t.Errorf("a failing CLI yields an empty version, got %q", got)
	}
}

func TestCLIVersionsSkipsMissingCLIsAndHonoursCancel(t *testing.T) {
	fx := newDiscoveryFixture(t, nil)
	if versions := fx.manager.CLIVersions(context.Background()); len(versions) != 0 {
		t.Errorf("missing CLIs have no version: %v", versions)
	}

	slow := fakeCLI(t, "gcloud", `sleep 5`)
	fx = newDiscoveryFixture(t, func(name string) (string, error) {
		if name == "gcloud" {
			return slow, nil
		}
		return "", exec.ErrNotFound
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	versions := fx.manager.CLIVersions(ctx)
	if time.Since(start) > 3*time.Second || versions[provider.GCP] != "" {
		t.Errorf("a hanging CLI must be cut off: %v after %s", versions, time.Since(start))
	}
}
