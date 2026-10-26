package tui

import (
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/manager"
)

func TestFeedbackWarnsRunningProcesses(t *testing.T) {
	svc := newMultiService()
	hint := `eval "$(gctx env claude-work)"`
	svc.switchRes = manager.SwitchResult{Account: "claude-work", Slot: "claude", ApplyHint: hint, RunningProcesses: 2}
	m := multiModel(t, svc, 128, 30)
	m.selected = "claude-work"
	out := screen(press(t, m, "enter"))
	if !strings.Contains(out, "⚠ 2 ") || !strings.Contains(out, "processes already running keep the previous account") {
		t.Errorf("no running-process warning:\n%s", out)
	}

	svc.switchRes.RunningProcesses = 0
	m = multiModel(t, svc, 128, 30)
	m.selected = "claude-work"
	if out = screen(press(t, m, "enter")); strings.Contains(out, "already running") {
		t.Errorf("warning without running processes:\n%s", out)
	}
}

func TestJumpToLinkedAccount(t *testing.T) {
	svc := newMultiService()
	for i, account := range svc.overview.Accounts {
		if account.Name == "claude-vertex" {
			svc.overview.Accounts[i].Linked = "cloudsuite"
		}
	}
	m := multiModel(t, svc, 128, 30)
	m.selected = "claude-vertex"
	if m = press(t, m, ">"); m.selected != "cloudsuite" {
		t.Fatalf("> selected %q", m.selected)
	}
	m.selected = "claude-work"
	if m = pressQuick(t, m, ">"); m.selected != "claude-work" || !strings.Contains(m.status.text, "not linked") {
		t.Fatalf("> on an unlinked account: %q\n%s", m.selected, m.status.text)
	}
}
