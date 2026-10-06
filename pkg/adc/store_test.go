package adc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) Store {
	root := t.TempDir()
	return Store{
		Dir:         filepath.Join(root, "gctx", "adc"),
		DefaultPath: filepath.Join(root, "gcloud", "application_default_credentials.json"),
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0700)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

const secretADC = `{"refresh_token":"SECRET-TOKEN","type":"authorized_user"}`

func TestSaveAndRestore(t *testing.T) {
	s := newStore(t)
	if _, err := s.Save("work"); err == nil || !strings.HasPrefix(err.Error(), "no ADC found at ") {
		t.Fatalf("missing default ADC: %v", err)
	}

	write(t, s.DefaultPath, secretADC)
	stored, err := s.Save("work")
	if err != nil {
		t.Fatal(err)
	}
	if stored != s.StoragePath("work") {
		t.Errorf("stored at %s", stored)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(stored)
		dirInfo, _ := os.Stat(s.Dir)
		if info.Mode().Perm() != 0600 || dirInfo.Mode().Perm() != 0700 {
			t.Errorf("modes %v %v", info.Mode().Perm(), dirInfo.Mode().Perm())
		}
	}

	write(t, s.DefaultPath, `{"other":"x"}`)
	if err := s.Restore("work"); err != nil {
		t.Fatal(err)
	}
	if equal, _ := FilesEqual(s.DefaultPath, stored); !equal {
		t.Error("restore did not copy stored ADC")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(s.DefaultPath)
		if info.Mode().Perm() != 0600 {
			t.Errorf("restored mode %v", info.Mode().Perm())
		}
	}
	if err := s.Restore("nope"); err == nil || err.Error() != "no saved ADC for account: nope" {
		t.Errorf("restore missing: %v", err)
	}
}

func TestValidateNeverLeaksContent(t *testing.T) {
	dir := t.TempDir()
	for _, content := range []string{
		`{"refresh_token": "1//abc`,
		`{"refresh_token": "1//abc"} trailing`,
		`["1//abc"]`,
		`1//abc`,
	} {
		path := filepath.Join(dir, "adc.json")
		write(t, path, content)
		err := Validate(path)
		if err == nil {
			t.Fatalf("%q: expected error", content)
		}
		if strings.Contains(err.Error(), "1//abc") || strings.Contains(err.Error(), "'1'") {
			t.Errorf("%q: error leaks content: %v", content, err)
		}
	}
	write(t, filepath.Join(dir, "ok.json"), secretADC)
	if err := Validate(filepath.Join(dir, "ok.json")); err != nil {
		t.Error(err)
	}
}

func TestRemoveIsBounded(t *testing.T) {
	s := newStore(t)
	canonical := s.StoragePath("x")
	write(t, canonical, secretADC)
	victim := filepath.Join(filepath.Dir(s.DefaultPath), "..", "id_ed25519")
	write(t, victim, "private key")
	write(t, s.DefaultPath, secretADC)

	outside, err := s.Remove("x", victim)
	if err != nil {
		t.Fatal(err)
	}
	if outside != victim {
		t.Errorf("outside=%q", outside)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Error("file outside the store was deleted")
	}
	if _, err := os.Stat(canonical); !os.IsNotExist(err) {
		t.Error("canonical stored ADC not deleted")
	}
	if _, err := os.Stat(s.DefaultPath); err != nil {
		t.Error("default ADC must never be deleted")
	}

	// A recorded path inside the store is removed too.
	legacy := filepath.Join(s.Dir, "old-name_adc.json")
	write(t, legacy, secretADC)
	if outside, err := s.Remove("x", legacy); err != nil || outside != "" {
		t.Fatal(outside, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("recorded in-store path not deleted")
	}

	if outside, err := s.Remove("x", s.Dir); outside != s.Dir || err != nil {
		t.Errorf("store dir itself is not inside the store: %q %v", outside, err)
	}
}

func TestSnapshotChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	before := Snapshot(path)
	write(t, path, "a")
	after := Snapshot(path)
	if !before.Changed(after) {
		t.Error("creation not detected")
	}
	if after.Changed(Snapshot(path)) {
		t.Error("unchanged file reported as changed")
	}
	later := time.Now().Add(time.Hour)
	os.Chtimes(path, later, later)
	if !after.Changed(Snapshot(path)) {
		t.Error("rewrite not detected")
	}
}
