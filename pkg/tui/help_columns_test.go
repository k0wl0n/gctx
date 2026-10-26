package tui

import (
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/provider"
)

func TestHelpKeysAreNeverTruncatedFrom100Columns(t *testing.T) {
	t.Setenv("GCTX_ASCII", "")
	for _, width := range []int{100, 120, 160} {
		svc := withTencent(newMultiService())
		svc.cliVersions = map[provider.ID]string{provider.GCP: "540.0.0", provider.AWS: "2.27.4", provider.Azure: "2.75.0"}
		m := press(t, mainModel(t, svc, goldenSize{width, 40}), "?")
		if out := screen(m); strings.Contains(out, "…") {
			t.Errorf("help at %d columns truncates text:\n%s", width, out)
		}
	}
}
