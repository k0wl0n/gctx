package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// submitRun opens the run prompt for name, types input and returns the
// runExec that would be launched (nil when nothing runs).
func submitRun(t *testing.T, m Model, name, input string) (Model, *runExec) {
	t.Helper()
	var launched *runExec
	m.exec = func(c tea.ExecCommand, _ tea.ExecCallback) tea.Cmd {
		launched, _ = c.(*runExec)
		return nil
	}
	m, _ = m.openRun(name)
	m.modal.(*runPrompt).input = input
	m, _ = m.modal.update(m, keyMsg("enter"))
	return m, launched
}

func TestRunPromptUsesRowProviderCLI(t *testing.T) {
	svc := newFakeService(multiOverview())
	m := multiModel(t, svc, 120, 40)
	m, _ = m.openRun("prod-aws")
	if view := m.modal.view(m); !strings.Contains(view, "aws > ") || strings.Contains(view, "gcloud") {
		t.Errorf("prompt for an AWS row:\n%s", view)
	}

	_, launched := submitRun(t, m, "prod-aws", "aws s3 ls")
	if launched == nil || strings.Join(launched.args, " ") != "s3 ls" || launched.cli != "aws" {
		t.Fatalf("launched %+v, want aws with args s3 ls", launched)
	}
	_, launched = submitRun(t, m, "prod-aws", "gcloud config list")
	if launched == nil || launched.args[0] != "gcloud" {
		t.Errorf("a leading gcloud token must stay for an AWS row: %+v", launched)
	}
}

func TestRunPromptBatchCheckUsesProviderBin(t *testing.T) {
	overview := multiOverview()
	for i := range overview.Providers {
		overview.Providers[i].CLI.Bin = `C:\bin\` + string(overview.Providers[i].ID) + ".cmd"
	}
	m := multiModel(t, newFakeService(overview), 120, 40)
	m.goos = "windows"
	m, launched := submitRun(t, m, "prod-aws", "s3 ls %PATH%")
	if launched != nil || !strings.Contains(m.modal.(*runPrompt).errMsg, "aws.cmd") {
		t.Fatalf("launched %+v, modal %+v", launched, m.modal)
	}
}
