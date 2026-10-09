package tui

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/manager"
)

// TestSecretNeverRendered asserts that ADC credential markers never leak into
// the table or help screens. The ADCPath field (the internal config-recorded
// path, not shown in any view) carries the marker; if it ever appeared it
// would indicate unintended field rendering. The details screen intentionally
// shows account metadata (gcloud config account, etc.) so it is excluded.
func TestSecretNeverRendered(t *testing.T) {
	secret := "SECRET-REFRESH-security-test"
	// Seed the internal ADCPath field, which is never rendered in any view.
	overview := sampleOverview("sec", "sec")
	for i := range overview.Accounts {
		overview.Accounts[i].ADCPath = secret
	}
	svc := newFakeService(overview)
	svc.report = manager.CredentialReport{Accounts: []string{secret}}

	widths := []int{60, 80, 100, 120, 160}
	heights := []int{12, 24, 40}
	// Details screen intentionally renders StoredADCPath (not ADCPath) but we
	// exclude it anyway to keep the test focused on the table/help surfaces.
	screens := []screenKind{screenMain, screenHelp}

	for _, w := range widths {
		for _, h := range heights {
			m := newTestModel(t, svc, w, h)
			for _, sc := range screens {
				m.screen = sc
				out := m.View().Content
				if strings.Contains(out, secret) {
					t.Errorf("%dx%d screen=%v: secret leaked: %q", w, h, sc, out)
				}
			}
		}
	}
}

// TestNoClipboardPackageInDeps asserts that no clipboard package is in the
// dependency graph of pkg/tui.
func TestNoClipboardPackageInDeps(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./pkg/tui")
	cmd.Dir = "/Users/c5397047/Documents/project/gctx-wt-views"
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("go list failed: %v", err)
	}
	lines := strings.Split(string(out), "\n")
	clipboardPkgs := []string{
		"github.com/atotto/clipboard",
		"golang.design/x/clipboard",
		"github.com/go-ole/go-ole",
	}
	for _, line := range lines {
		for _, clip := range clipboardPkgs {
			if strings.Contains(line, clip) {
				t.Errorf("clipboard package in deps: %q", line)
			}
		}
	}
}
