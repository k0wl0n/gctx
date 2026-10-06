package manager

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAccountName(t *testing.T) {
	for _, name := range []string{"a", "work-2", "my-account"} {
		if err := ValidateAccountName(name); err != nil {
			t.Errorf("%q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", "Work", "1a", "a b", "../x", "a/b", `a\b`, "a.b", "a_b"} {
		err := ValidateAccountName(name)
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("%q accepted or wrong error: %v", name, err)
		}
	}
}

func TestValidateProjectID(t *testing.T) {
	for _, id := range []string{"my-proj-1", "example.com:my-proj", "demo-project", "abcde1"} {
		if err := ValidateProjectID(id); err != nil {
			t.Errorf("%q rejected: %v", id, err)
		}
	}
	invalid := []string{"", "ab", "My-proj", "proj-", strings.Repeat("a", 31), "1abcdef", "bad_domain:my-proj", "example.com:ab", ":my-proj"}
	for _, id := range invalid {
		if err := ValidateProjectID(id); !errors.Is(err, ErrInvalidProjectID) {
			t.Errorf("%q accepted or wrong error: %v", id, err)
		}
	}
}

func TestResolvePaths(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(key string) string { return vars[key] }
	}
	tests := []struct {
		name       string
		goos, home string
		vars       map[string]string
		gcloudDir  string
	}{
		{"linux", "linux", "/home/u", nil, "/home/u/.config/gcloud"},
		{"darwin", "darwin", "/Users/u", nil, "/Users/u/.config/gcloud"},
		{"linux custom", "linux", "/home/u", map[string]string{"CLOUDSDK_CONFIG": "/tmp/gc"}, "/tmp/gc"},
		{"windows appdata", "windows", `C:\Users\u`, map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}, `C:\Users\u\AppData\Roaming/gcloud`},
		{"windows custom", "windows", `C:\Users\u`, map[string]string{"CLOUDSDK_CONFIG": `D:\gc`, "APPDATA": `C:\x`}, `D:\gc`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := ResolvePaths(env(tt.vars), tt.goos, tt.home)
			if filepathSlash(paths.GcloudDir) != filepathSlash(tt.gcloudDir) {
				t.Errorf("GcloudDir %q, want %q", paths.GcloudDir, tt.gcloudDir)
			}
			if !strings.HasPrefix(filepathSlash(paths.DefaultADC), filepathSlash(paths.GcloudDir)) ||
				!strings.HasSuffix(paths.DefaultADC, "application_default_credentials.json") {
				t.Errorf("DefaultADC %q", paths.DefaultADC)
			}
			if filepathSlash(paths.GctxDir) != filepathSlash(tt.home+"/.config/gctx") {
				t.Errorf("GctxDir %q", paths.GctxDir)
			}
			if filepathSlash(paths.ADCDir) != filepathSlash(paths.GctxDir+"/adc") {
				t.Errorf("ADCDir %q", paths.ADCDir)
			}
		})
	}
}

func filepathSlash(s string) string { return strings.ReplaceAll(s, `\`, "/") }
