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

func TestDefaultSetRegistersAWS(t *testing.T) {
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
			if name == "aws" {
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

	awsAccount, ok := overview.Account("a-aws")
	if !ok || awsAccount.Provider != provider.AWS || awsAccount.TargetLabel != "PROFILE" || awsAccount.Status != provider.HealthCLIMissing {
		t.Fatalf("aws account %+v", awsAccount)
	}
	gcpAccount, _ := overview.Account("g")
	if gcpAccount.Provider != provider.GCP || gcpAccount.Status == provider.HealthCLIMissing {
		t.Fatalf("gcp account %+v", gcpAccount)
	}
}
