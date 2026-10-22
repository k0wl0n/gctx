package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLegacyActiveAccountBecomesGCPSlot(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t, legacyFixture))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ActiveBySlot["gcp"]; got != "work" {
		t.Errorf("gcp slot = %q, want work", got)
	}
}

func TestSaveMirrorsGCPSlotToLegacyField(t *testing.T) {
	path := writeConfig(t, legacyFixture)
	cfg, _ := LoadFrom(path)
	cfg.Accounts["prod"] = &Account{Name: "prod", Provider: "aws"}
	if err := cfg.SetActiveSlot("aws", "prod"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetActive("work"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, `"active_account": "work"`) {
		t.Errorf("legacy field missing:\n%s", text)
	}
	if !strings.Contains(text, `"aws": "prod"`) || strings.Contains(text, `"gcp":`) {
		t.Errorf("active_by_slot should hold aws only:\n%s", text)
	}
}

func TestGCPOnlyConfigStaysByteIdentical(t *testing.T) {
	path := writeConfig(t, legacyFixture)
	cfg, _ := LoadFrom(path)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != legacyFixture {
		t.Errorf("bytes changed:\n%s", data)
	}
}

func TestDeleteClearsEverySlotPointingAtAccount(t *testing.T) {
	cfg, _ := LoadFrom(writeConfig(t, legacyFixture))
	if err := cfg.SetActiveSlot("openai-api", "work"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.DeleteAccount("work"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.ActiveBySlot) != 0 || cfg.ActiveAccount != "" {
		t.Errorf("slots not cleared: %v %q", cfg.ActiveBySlot, cfg.ActiveAccount)
	}
}
