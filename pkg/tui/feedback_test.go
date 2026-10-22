package tui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
)

const wantEval = `eval "$(` + config.AppName + ` env legacy-aws)"`

// awsSwitchModel selects legacy-aws on the AWS tab of the multi-provider
// fixture; the fake service answers a switch like an env-based provider.
func awsSwitchModel(t testing.TB, replaced string) Model {
	t.Helper()
	svc := newMultiService()
	svc.switchRes = manager.SwitchResult{Account: "legacy-aws", Slot: "aws", Replaced: replaced, ApplyHint: wantEval}
	m := multiModel(t, svc, 128, 30)
	m = m.selectTab(2)
	m.selected = "legacy-aws"
	return m
}

func TestEnvSwitchNeverClaimsShellChanged(t *testing.T) {
	m := press(t, awsSwitchModel(t, "prod-aws"), "enter")
	out := screen(m)
	if !strings.Contains(out, "unchanged") || !strings.Contains(out, "$ "+wantEval) {
		t.Errorf("panel lacks the unchanged note or eval line:\n%s", out)
	}
	for _, claim := range []string{"switched your shell", "shell is now", "terminal is now"} {
		if strings.Contains(strings.ToLower(out), claim) {
			t.Errorf("screen claims %q", claim)
		}
	}
	for _, action := range []string{"copy that line", "open subshell", "run here", "esc dismiss"} {
		if !strings.Contains(out, action) {
			t.Errorf("panel lacks %q", action)
		}
	}
}

func TestFeedbackNamesReplacedAccount(t *testing.T) {
	out := screen(press(t, awsSwitchModel(t, "prod-aws"), "enter"))
	if !strings.Contains(out, "AWS active account is now legacy-aws (was prod-aws)") {
		t.Errorf("no replaced account:\n%s", out)
	}
	out = screen(press(t, awsSwitchModel(t, ""), "enter"))
	if strings.Contains(out, "(was ") {
		t.Errorf("replaced shown for an unchanged slot:\n%s", out)
	}
}

func TestFeedbackDismissedByEsc(t *testing.T) {
	m := press(t, awsSwitchModel(t, ""), "enter")
	if m.feedback == nil {
		t.Fatal("no panel after enter")
	}
	m = press(t, m, "esc")
	if m.feedback != nil || strings.Contains(screen(m), "unchanged") {
		t.Error("panel still open after esc")
	}
}

func TestGCPSwitchKeepsGlobalSemantics(t *testing.T) {
	svc := newMultiService()
	svc.switchRes = manager.SwitchResult{Account: "cloudsuite", Slot: "gcp"}
	m := multiModel(t, svc, 128, 30)
	m.selected = "cloudsuite"
	m, _ = m.startSwitch("cloudsuite")
	m, _ = m.handleOpDone(opDoneMsg{seq: m.op.seq, kind: opSwitch, account: "cloudsuite", result: svc.switchRes})
	if m.feedback != nil || strings.Contains(screen(m), "unchanged") {
		t.Error("GCP switch opened a feedback panel")
	}
	if m.status.text != "switched to cloudsuite" {
		t.Errorf("status %q", m.status.text)
	}
}

func TestCopyEnvEmitsClipboardCmd(t *testing.T) {
	m := awsSwitchModel(t, "")
	_, cmd := m.dispatch(keyMsg("y"))
	if cmd == nil {
		t.Fatal("no command")
	}
	want := tea.SetClipboard(wantEval)()
	var found bool
	var walk func(tea.Msg)
	walk = func(msg tea.Msg) {
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c == nil {
					continue
				}
				done := make(chan tea.Msg, 1)
				go func() { done <- c() }()
				select { // skip the status expiry timer
				case result := <-done:
					walk(result)
				case <-time.After(100 * time.Millisecond):
				}
			}
			return
		}
		found = found || msg == want
	}
	walk(cmd())
	if !found {
		t.Error("no clipboard command for the eval line")
	}
	m, _ = m.dispatch(keyMsg("y"))
	if !strings.Contains(m.status.text, wantEval) {
		t.Errorf("status %q", m.status.text)
	}
}

func TestSubshellKeysPassScope(t *testing.T) {
	for key, want := range map[string]manager.ShellScope{"s": manager.ShellAllActive, "S": manager.ShellOnly} {
		svc := newMultiService()
		svc.shellCmd = exec.Command("true")
		m := multiModel(t, svc, 128, 30)
		m.exec = execSync("", nil)
		m.selected = "legacy-aws"
		_, cmd := m.dispatch(keyMsg(key))
		cmd()
		if len(svc.shellScopes) != 1 || svc.shellScopes[0] != want {
			t.Errorf("%s: scopes %v, want %v", key, svc.shellScopes, want)
		}
	}
}

func TestHintAndHelpListCopyAndSubshellKeys(t *testing.T) {
	m := multiModel(t, newMultiService(), 70, 24)
	for _, line := range lines(screen(m)) {
		if w := ansi.StringWidth(line); w > 70 {
			t.Errorf("line is %d wide: %q", w, line)
		}
	}
	if !strings.Contains(screen(multiModel(t, newMultiService(), 120, 24)), "y copy env") {
		t.Error("hint line lacks the copy key")
	}
	m.screen = screenHelp
	help := screen(m)
	for _, want := range []string{"copy the env eval line", "subshell with only this account", "subshell with every active slot"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}
