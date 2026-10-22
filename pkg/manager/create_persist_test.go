package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// defaultSetManager builds a manager over the default provider set, with
// every CLI reported as installed.
func defaultSetManager(t *testing.T, env map[string]string) (*manager.Manager, manager.Paths) {
	t.Helper()
	dir := t.TempDir()
	paths := manager.Paths{
		GctxDir:    filepath.Join(dir, "gctx"),
		ADCDir:     filepath.Join(dir, "gctx", "adc"),
		GcloudDir:  filepath.Join(dir, "gcloud"),
		DefaultADC: filepath.Join(dir, "gcloud", "application_default_credentials.json"),
	}
	m, err := manager.NewWithOptions(manager.Options{
		Paths:    paths,
		Getenv:   func(key string) string { return env[key] },
		Gcloud:   &gcloud.Client{Bin: "/usr/bin/gcloud"},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, paths
}

func storedAccount(t *testing.T, paths manager.Paths, name string) *config.Account {
	t.Helper()
	cfg, err := config.LoadFrom(paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	stored := cfg.Accounts[name]
	if stored == nil {
		t.Fatalf("account %q not saved", name)
	}
	return stored
}

func TestCreateAccountPersistsAzureDir(t *testing.T) {
	m, paths := defaultSetManager(t, nil)
	_, err := m.CreateAccount(context.Background(), manager.CreateRequest{
		Provider: provider.Azure, Name: "work", Target: "12345678-1234-1234-1234-123456789abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(paths.GctxDir, "azure", "work")
	stored := storedAccount(t, paths, "work")
	if stored.Settings["dir"] != wantDir {
		t.Fatalf("saved settings %v, want dir %q", stored.Settings, wantDir)
	}
	if info, err := os.Stat(wantDir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir %v err %v", info, err)
	}

	_, err = m.CreateAccount(context.Background(), manager.CreateRequest{
		Provider: provider.Azure, Name: "tenanted", Target: "12345678-1234-1234-1234-123456789abc",
		Fields: map[string]string{"tenant": "contoso.onmicrosoft.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := storedAccount(t, paths, "tenanted").Settings; got["tenant"] != "contoso.onmicrosoft.com" || got["dir"] == "" {
		t.Fatalf("saved settings %v", got)
	}
}

func TestCreateAccountPersistsAWSSettings(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "aws-config")
	if err := os.WriteFile(configFile, []byte("[profile prod]\nregion = eu-west-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, paths := defaultSetManager(t, map[string]string{"AWS_CONFIG_FILE": configFile})
	_, err := m.CreateAccount(context.Background(), manager.CreateRequest{
		Provider: provider.AWS, Name: "prod", Target: "prod",
		Fields: map[string]string{"auth": "profile", "region": "eu-west-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := storedAccount(t, paths, "prod").Settings
	if got["auth"] != "profile" || got["region"] != "eu-west-1" {
		t.Fatalf("saved settings %v", got)
	}
}

func TestCreateAccountGCPKeepsNoSettings(t *testing.T) {
	gcpFake := newFake(provider.GCP, "PROJECT")
	m, configPath := dispatchManager(t, nil, gcpFake)
	if _, err := m.CreateAccount(context.Background(), manager.CreateRequest{Name: "work", Target: "p1"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, `"settings"`) {
		t.Fatalf("gcp account JSON gained settings: %s", got)
	}
}
