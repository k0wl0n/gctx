package cmd

import "testing"

func TestShouldLaunchTUI(t *testing.T) {
	tests := []struct {
		name                string
		env                 map[string]string
		stdinTTY, stdoutTTY bool
		want                bool
	}{
		{"interactive", map[string]string{"TERM": "xterm-256color"}, true, true, true},
		{"stdin piped", nil, false, true, false},
		{"stdout redirected", nil, true, false, false},
		{"dumb terminal", map[string]string{"TERM": "dumb"}, true, true, false},
		{"CI true", map[string]string{"CI": "true"}, true, true, false},
		{"CI 1", map[string]string{"CI": "1"}, true, true, false},
		{"CI false", map[string]string{"CI": "false"}, true, true, true},
		{"CI 0", map[string]string{"CI": "0"}, true, true, true},
		{"CI empty", map[string]string{"CI": ""}, true, true, true},
		{"opt out", map[string]string{"GCTX_NO_TUI": "1"}, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			if got := shouldLaunchTUI(getenv, tt.stdinTTY, tt.stdoutTTY); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
