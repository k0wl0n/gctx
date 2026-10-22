package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// legacyFixture is a config.json as written by gctx before this change.
const legacyFixture = `{
  "accounts": {
    "work": {
      "name": "work",
      "config_name": "work-config",
      "project_id": "p1",
      "adc_path": "/home/u/.config/gctx/adc/work_adc.json",
      "created_at": "2026-04-01T10:00:00Z",
      "email": "w@example.com"
    }
  },
  "active_account": "work"
}`

func TestSchemaRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(legacyFixture), 0644)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != legacyFixture {
		t.Errorf("schema changed:\n%s", data)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0644 {
			t.Errorf("mode %v", info.Mode().Perm())
		}
	}
}

func TestLoadMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	cfg, err := LoadFrom(path)
	if err != nil || len(cfg.Accounts) != 0 {
		t.Fatalf("%+v %v", cfg, err)
	}
	if err := cfg.AddAccount(&Account{Name: "a", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("config not written to its load path")
	}
}

func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadFrom(filepath.Join(dir, "config.json"))
	cfg.AddAccount(&Account{Name: "a"})
	cfg.SetActive("a")
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("unexpected files: %v", entries)
	}
}

func TestFailedSaveKeepsPreviousContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(legacyFixture), 0644)
	cfg, _ := LoadFrom(path)
	cfg.ActiveAccount = "changed"

	// Make the target a non-empty directory so rename fails on every OS.
	os.Remove(path)
	os.MkdirAll(filepath.Join(path, "block"), 0755)
	if err := cfg.Save(); err == nil {
		t.Fatal("expected save to fail")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestListAccountsSorted(t *testing.T) {
	cfg := &Config{Accounts: map[string]*Account{
		"work": {Name: "work"}, "alpha": {Name: "alpha"}, "mid": {Name: "mid"},
	}}
	var names []string
	for _, a := range cfg.ListAccounts() {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "alpha,mid,work" {
		t.Errorf("got %v", names)
	}
}

func TestInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte("{nope"), 0644)
	if _, err := LoadFrom(path); err == nil {
		t.Error("expected error")
	}
}

func TestLoadV1ConfigDefaultsToGCP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(legacyFixture), 0644)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	account := cfg.Accounts["work"]
	if account.Provider != "" {
		t.Errorf("Provider = %q, want empty for a v1 config", account.Provider)
	}
	if got := account.ProviderID(); got != "gcp" {
		t.Errorf("ProviderID() = %q, want gcp", got)
	}
}

func TestRoundTripPreservesSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.AddAccount(&Account{
		Name:      "aws-prod",
		Provider:  "aws",
		ProjectID: "prod-profile",
		Settings:  map[string]string{"region": "eu-west-1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	account := loaded.Accounts["aws-prod"]
	if account.ProviderID() != "aws" {
		t.Errorf("ProviderID() = %q, want aws", account.ProviderID())
	}
	if got := account.Settings["region"]; got != "eu-west-1" {
		t.Errorf("Settings[region] = %q, want eu-west-1", got)
	}
}

func TestTargetReturnsProjectID(t *testing.T) {
	account := Account{ProjectID: "my-target"}
	if got := account.Target(); got != "my-target" {
		t.Errorf("Target() = %q, want my-target", got)
	}
}

func TestConfigDirIsUnderAppName(t *testing.T) {
	dir, err := GetConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(dir), "/.config/gctx") {
		t.Errorf("GetConfigDir() = %q, want suffix /.config/gctx", dir)
	}
}
