package tui

import "strings"

// tokenize splits run-prompt input into gcloud arguments without a shell.
// Stub: whitespace split.
func tokenize(input, gcloudBin, goos string) ([]string, error) {
	return strings.Fields(input), nil
}
