package gcloudconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDir(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	home := filepath.Join("h", "u")
	tests := []struct {
		name string
		vars map[string]string
		goos string
		want string
	}{
		{"override", map[string]string{"CLOUDSDK_CONFIG": "custom"}, "linux", "custom"},
		{"linux", nil, "linux", filepath.Join(home, ".config", "gcloud")},
		{"darwin", nil, "darwin", filepath.Join(home, ".config", "gcloud")},
		{"windows appdata", map[string]string{"APPDATA": "ad"}, "windows", filepath.Join("ad", "gcloud")},
		{"windows fallback", nil, "windows", filepath.Join(home, "AppData", "Roaming", "gcloud")},
		{"windows override", map[string]string{"CLOUDSDK_CONFIG": "c", "APPDATA": "ad"}, "windows", "c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Dir(env(tt.vars), tt.goos, home); got != tt.want {
				t.Errorf("Dir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestActiveConfig(t *testing.T) {
	dir := t.TempDir()
	if got := ActiveConfig(dir); got != "default" {
		t.Errorf("missing file: %q", got)
	}
	os.WriteFile(filepath.Join(dir, "active_config"), []byte("work-config\n"), 0644)
	if got := ActiveConfig(dir); got != "work-config" {
		t.Errorf("got %q", got)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Read(filepath.Join(dir, "missing"), "x")
	if err != nil || cfg.Exists {
		t.Fatalf("missing dir: %+v %v", cfg, err)
	}

	os.MkdirAll(filepath.Join(dir, "configurations"), 0755)
	content := "# comment\n[compute]\nproject = not-this\n\n[core]\n  account =  w@example.com \n; note\nproject=p1\nbroken line\n"
	os.WriteFile(Path(dir, "work"), []byte(content), 0644)
	cfg, err = Read(dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Exists || cfg.Account != "w@example.com" || cfg.Project != "p1" {
		t.Errorf("got %+v", cfg)
	}

	os.WriteFile(Path(dir, "empty"), nil, 0644)
	cfg, err = Read(dir, "empty")
	if err != nil || !cfg.Exists || cfg.Account != "" {
		t.Errorf("empty file: %+v %v", cfg, err)
	}
}

func TestReadUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory-as-file read error is POSIX-specific")
	}
	dir := t.TempDir()
	os.MkdirAll(Path(dir, "weird"), 0755) // a directory where a file is expected
	if _, err := Read(dir, "weird"); err == nil {
		t.Error("expected error")
	}
}
