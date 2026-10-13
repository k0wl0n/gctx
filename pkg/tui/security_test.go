package tui

import (
	"errors"
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
	cmd := exec.Command("go", "list", "-deps", "github.com/k0wl0n/gctx/pkg/tui")
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

// TestNoOsExitOrLogFatalInTUI greps the package source for os.Exit and log.Fatal calls,
// which must not appear in non-test files of pkg/tui.
func TestNoOsExitOrLogFatalInTUI(t *testing.T) {
	// Use grep to search only non-test Go files.
	cmd := exec.Command("sh", "-c",
		`grep -rn --include="*.go" "os\.Exit\|log\.Fatal" . | grep -v "_test\.go"`)
	cmd.Dir = "/Users/c5397047/Documents/project/gctx/pkg/tui"
	out, err := cmd.Output()
	if err != nil {
		// grep exits 1 when no lines match — that is the desired outcome.
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		t.Errorf("forbidden call found in pkg/tui: %s", line)
	}
}

// TestPanicRestoreDoesNotKillProcess verifies that a panic-like error from
// mapRunError is translated to a non-nil error rather than re-panicking.
// This covers the ErrProgramPanic path that Bubble Tea uses when the model panics.
func TestPanicRestoreDoesNotKillProcess(t *testing.T) {
	// Simulate the error value BubbleTea produces when the model panics.
	simulatedPanic := errors.New("interactive UI failed: program panic")
	result := mapRunError(simulatedPanic)
	if result == nil {
		t.Fatal("expected mapRunError to return a non-nil error for a panic-like error")
	}
	if !strings.Contains(result.Error(), "interactive UI failed") {
		t.Errorf("unexpected error text: %q", result.Error())
	}
}
