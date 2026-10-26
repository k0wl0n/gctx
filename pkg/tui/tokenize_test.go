package tui

import (
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		gcloudBin string
		goos      string
		want      []string
		wantErr   string
	}{
		// Basic whitespace splitting.
		{
			name:      "simple args",
			input:     "compute instances list",
			gcloudBin: "gcloud",
			goos:      "linux",
			want:      []string{"compute", "instances", "list"},
		},
		// Leading gcloud token dropped.
		{
			name:      "strip leading gcloud",
			input:     "gcloud compute instances list",
			gcloudBin: "gcloud",
			goos:      "linux",
			want:      []string{"compute", "instances", "list"},
		},
		// Credential-safety scenario: shell metacharacters rejected.
		{
			name:      "metachar semicolon",
			input:     "compute instances list; rm -rf ~",
			gcloudBin: "gcloud",
			goos:      "linux",
			wantErr:   "Shell operators are not supported here; press s for an isolated shell",
		},
		// Credential-safety scenario: quoted filter preserved.
		{
			name:      "double-quoted filter",
			input:     `compute instances list --filter="name~'^web' AND status=RUNNING"`,
			gcloudBin: "gcloud",
			goos:      "linux",
			want:      []string{"compute", "instances", "list", "--filter=name~'^web' AND status=RUNNING"},
		},
		// Credential-safety scenario: Windows backslash path.
		{
			name:      "windows backslash path",
			input:     `storage cp C:\tmp\a.txt gs://b/`,
			gcloudBin: "gcloud",
			goos:      "linux",
			want:      []string{"storage", "cp", `C:\tmp\a.txt`, "gs://b/"},
		},
		// Pipe character.
		{
			name:      "pipe rejected",
			input:     "auth list | grep active",
			gcloudBin: "gcloud",
			goos:      "linux",
			wantErr:   "Shell operators are not supported here; press s for an isolated shell",
		},
		// Ampersand.
		{
			name:      "ampersand rejected",
			input:     "auth list & echo done",
			gcloudBin: "gcloud",
			goos:      "linux",
			wantErr:   "Shell operators are not supported here; press s for an isolated shell",
		},
		// Backtick.
		{
			name:      "backtick rejected",
			input:     "config set project `echo x`",
			gcloudBin: "gcloud",
			goos:      "linux",
			wantErr:   "Shell operators are not supported here; press s for an isolated shell",
		},
		// $( rejected.
		{
			name:      "dollar-paren rejected",
			input:     "config set project $(cat /etc/passwd)",
			gcloudBin: "gcloud",
			goos:      "linux",
			wantErr:   "Shell operators are not supported here; press s for an isolated shell",
		},
		// Redirect rejected.
		{
			name:      "redirect rejected",
			input:     "auth list > /tmp/out",
			gcloudBin: "gcloud",
			goos:      "linux",
			wantErr:   "Shell operators are not supported here; press s for an isolated shell",
		},
		// Unterminated single quote.
		{
			name:      "unterminated single quote",
			input:     "compute 'instances",
			gcloudBin: "gcloud",
			goos:      "linux",
			wantErr:   "unterminated single quote",
		},
		// Unterminated double quote.
		{
			name:      "unterminated double quote",
			input:     `compute "instances`,
			gcloudBin: "gcloud",
			goos:      "linux",
			wantErr:   "unterminated double quote",
		},
		// Single-quoted string.
		{
			name:      "single-quoted arg",
			input:     "config set project 'my project'",
			gcloudBin: "gcloud",
			goos:      "linux",
			want:      []string{"config", "set", "project", "my project"},
		},
		// Backslash-escaped space.
		{
			name:      "backslash space",
			input:     `config set project my\ project`,
			gcloudBin: "gcloud",
			goos:      "linux",
			want:      []string{"config", "set", "project", "my project"},
		},
		// Backslash before non-special char is literal (both chars kept).
		{
			name:      "backslash non-special",
			input:     `storage cp C:\tmp`,
			gcloudBin: "gcloud",
			goos:      "linux",
			want:      []string{"storage", "cp", `C:\tmp`},
		},
		// Empty input.
		{
			name:      "empty input",
			input:     "",
			gcloudBin: "gcloud",
			goos:      "linux",
			want:      nil,
		},
		// Windows .cmd with percent-sign argument.
		{
			name:      "windows cmd percent-sign",
			input:     "config set core/project %USERPROFILE%",
			gcloudBin: "gcloud.cmd",
			goos:      "windows",
			wantErr:   "unsafe with gcloud.cmd",
		},
		// Windows .cmd with safe args: OK.
		{
			name:      "windows cmd safe args",
			input:     "config list",
			gcloudBin: "gcloud.cmd",
			goos:      "windows",
			want:      []string{"config", "list"},
		},
		// .bat extension also triggers Windows check.
		{
			name:      "windows bat ampersand",
			input:     "config list",
			gcloudBin: "gcloud.bat",
			goos:      "windows",
			want:      []string{"config", "list"},
		},
		// Non-windows with .cmd ext: no Windows check (goos matters).
		{
			name:      "non-windows cmd ext",
			input:     "config set p %VAR%",
			gcloudBin: "gcloud.cmd",
			goos:      "linux",
			want:      []string{"config", "set", "p", "%VAR%"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tokenize(tc.input, "gcloud", tc.gcloudBin, tc.goos)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("arg[%d]: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
