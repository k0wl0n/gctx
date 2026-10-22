package manager_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestDefaultSetRegistersAzure(t *testing.T) {
	dir := t.TempDir()
	paths := manager.Paths{
		GctxDir:    filepath.Join(dir, "gctx"),
		ADCDir:     filepath.Join(dir, "gctx", "adc"),
		GcloudDir:  filepath.Join(dir, "gcloud"),
		DefaultADC: filepath.Join(dir, "gcloud", "application_default_credentials.json"),
	}
	cfg, err := config.LoadFrom(paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, acct := range []config.Account{
		{Name: "a-azure", ProjectID: "12345678-1234-1234-1234-123456789abc", Provider: "azure"},
		{Name: "a-aws", ProjectID: "prod", Provider: "aws"},
		{Name: "g", ProjectID: "my-project", ConfigName: "g-config"},
	} {
		if err := cfg.AddAccount(&acct); err != nil {
			t.Fatal(err)
		}
	}

	m, err := manager.NewWithOptions(manager.Options{
		Paths:  paths,
		Getenv: func(string) string { return "" },
		Gcloud: &gcloud.Client{Bin: "/usr/bin/gcloud"},
		LookPath: func(name string) (string, error) {
			if name == "az" {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}

	azureAccount, ok := overview.Account("a-azure")
	if !ok || azureAccount.Provider != provider.Azure || azureAccount.TargetLabel != "SUBSCRIPTION" || azureAccount.Status != provider.HealthCLIMissing {
		t.Fatalf("azure account %+v", azureAccount)
	}
	if awsAccount, _ := overview.Account("a-aws"); awsAccount.Status == provider.HealthCLIMissing {
		t.Fatalf("aws row affected: %+v", awsAccount)
	}
	if gcpAccount, _ := overview.Account("g"); gcpAccount.Status == provider.HealthCLIMissing {
		t.Fatalf("gcp row affected: %+v", gcpAccount)
	}
}
