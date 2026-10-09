package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
)

// TestLoginExecSuccess tests a successful login exec flow at model level.
func TestLoginExecSuccess(t *testing.T) {
	svc := newFakeService(sampleOverview("sandbox", "sandbox", "work"))
	svc.loginRes = manager.LoginResult{Account: "sandbox", ADCSaved: true, GcloudLoggedIn: true}
	m := newTestModel(t, svc, 100, 24)

	var out bytes.Buffer
	m.exec = execSync("\n", &out)

	m, cmd := m.beginLogin("sandbox")
	m = drain(t, m, cmd)
	_ = m
}

// TestLoginExecFailure tests failed login at exec level.
func TestLoginExecFailure(t *testing.T) {
	svc := newFakeService(sampleOverview("sandbox", "sandbox"))
	svc.loginErr = fmt.Errorf("cancelled")
	m := newTestModel(t, svc, 100, 24)
	m.exec = execSync("\n", nil)
	m, cmd := m.beginLogin("sandbox")
	m = drain(t, m, cmd)
	_ = m
}

// TestHandleExecDoneLoginSuccess tests status message for login success.
func TestHandleExecDoneLoginSuccess(t *testing.T) {
	svc := newFakeService(sampleOverview("sandbox", "sandbox"))
	m := newTestModel(t, svc, 100, 24)
	msg := execDoneMsg{
		kind:    execLogin,
		account: "sandbox",
		err:     &loginError{err: nil, result: manager.LoginResult{Account: "sandbox", ADCSaved: true}},
	}
	m, cmd := m.handleExecDone(msg)
	_ = drain(t, m, cmd)
	if !strings.Contains(m.status.text, "Login complete for sandbox") {
		t.Errorf("expected login complete status, got %q", m.status.text)
	}
}

// TestHandleExecDoneLoginCancel tests status for cancelled auth login step.
func TestHandleExecDoneLoginCancel(t *testing.T) {
	svc := newFakeService(sampleOverview("sandbox", "sandbox"))
	m := newTestModel(t, svc, 100, 24)
	msg := execDoneMsg{
		kind:    execLogin,
		account: "sandbox",
		err:     &loginError{err: &gcloud.StepError{Step: manager.StepAuthLogin, Err: fmt.Errorf("cancelled")}, result: manager.LoginResult{Account: "sandbox"}},
	}
	m, _ = m.handleExecDone(msg)
	if !strings.Contains(m.status.text, "Login cancelled for sandbox") {
		t.Errorf("expected cancelled status, got %q", m.status.text)
	}
}

// TestHandleExecDoneLoginADCFailed tests status when ADC step fails.
func TestHandleExecDoneLoginADCFailed(t *testing.T) {
	svc := newFakeService(sampleOverview("sandbox", "sandbox"))
	m := newTestModel(t, svc, 100, 24)
	msg := execDoneMsg{
		kind:    execLogin,
		account: "sandbox",
		err:     &loginError{err: &gcloud.StepError{Step: manager.StepADCLogin, Err: fmt.Errorf("timeout")}, result: manager.LoginResult{Account: "sandbox", GcloudLoggedIn: true}},
	}
	m, _ = m.handleExecDone(msg)
	if !strings.Contains(m.status.text, "ADC was not saved") {
		t.Errorf("expected ADC not saved status, got %q", m.status.text)
	}
}

// TestHandleExecDoneShell tests shell exit messages.
func TestHandleExecDoneShell(t *testing.T) {
	svc := newFakeService(sampleOverview("personal", "personal"))
	m := newTestModel(t, svc, 100, 24)

	// Clean exit.
	msg := execDoneMsg{kind: execShell, account: "personal", err: &shellError{exitCode: 0}}
	m, _ = m.handleExecDone(msg)
	if !strings.Contains(m.status.text, "Shell for personal exited") {
		t.Errorf("got %q", m.status.text)
	}

	// Non-zero exit.
	msg = execDoneMsg{kind: execShell, account: "personal", err: &shellError{exitCode: 1}}
	m, _ = m.handleExecDone(msg)
	if !strings.Contains(m.status.text, "status 1") {
		t.Errorf("expected status 1 in message, got %q", m.status.text)
	}
}

// TestHandleExecDoneRun tests run exit messages.
func TestHandleExecDoneRun(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)

	msg := execDoneMsg{kind: execRun, account: "work", err: &runError{exitCode: 0}}
	m, _ = m.handleExecDone(msg)
	if !strings.Contains(m.status.text, "status 0") {
		t.Errorf("got %q", m.status.text)
	}

	msg = execDoneMsg{kind: execRun, account: "work", err: &runError{exitCode: 2}}
	m, _ = m.handleExecDone(msg)
	if m.status.level != levelError {
		t.Error("expected error level for non-zero exit")
	}
}

// TestRunPromptHistory tests up/down history navigation.
func TestRunPromptHistory(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m.exec = execSync("\n", nil)

	// Open run prompt.
	m, _ = m.openRun("work")
	m.history = []string{"config list", "auth list"}
	p := m.modal.(*runPrompt)
	p.historyPos = -1

	// Press up to navigate history.
	m = press(t, m, "up")
	p = m.modal.(*runPrompt)
	if p.input != "auth list" {
		t.Errorf("expected 'auth list', got %q", p.input)
	}

	m = press(t, m, "up")
	p = m.modal.(*runPrompt)
	if p.input != "config list" {
		t.Errorf("expected 'config list', got %q", p.input)
	}

	// Press down to go back.
	m = press(t, m, "down")
	p = m.modal.(*runPrompt)
	if p.input != "auth list" {
		t.Errorf("expected 'auth list' going down, got %q", p.input)
	}
}

// TestRunPromptEmptyNotSubmitted tests that empty input is ignored.
func TestRunPromptEmptyNotSubmitted(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.openRun("work")
	calls := len(svc.Calls())
	m = press(t, m, "enter")
	if m.modal == nil {
		t.Error("modal should remain open after empty submit")
	}
	if len(svc.Calls()) != calls {
		t.Error("RunCommand should not be called on empty input")
	}
}

// TestRunPromptMetacharError tests inline tokenizer error display.
func TestRunPromptMetacharError(t *testing.T) {
	svc := newFakeService(sampleOverview("work", "work"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.openRun("work")
	p := m.modal.(*runPrompt)
	p.input = "auth list | grep active"

	// Dispatch enter.
	var cmd tea.Cmd
	m, cmd = m.modal.update(m, keyMsg("enter"))
	_ = cmd
	if m.modal == nil {
		t.Error("modal should stay open on error")
	}
	if m.modal != nil {
		p2 := m.modal.(*runPrompt)
		if !strings.Contains(p2.errMsg, "Shell operators") {
			t.Errorf("expected shell operators error, got %q", p2.errMsg)
		}
	}
}

// TestBeginShell tests the shell exec seam.
func TestBeginShell(t *testing.T) {
	svc := newFakeService(sampleOverview("personal", "personal"))
	// Use a shell command that exits immediately.
	svc.shellCmd = exec.Command(os.Args[0], "-test.run=NOTEXIST")
	m := newTestModel(t, svc, 100, 24)
	m.exec = execSync("", nil)
	m, cmd := m.beginShell("personal")
	m = drain(t, m, cmd)
	_ = m
	found := false
	for _, c := range svc.Calls() {
		if strings.HasPrefix(c, "ShellCommand") {
			found = true
		}
	}
	if !found {
		t.Error("expected ShellCommand to be called")
	}
}
