package tui

import (
	"io"

	tea "charm.land/bubbletea/v2"
)

type execKind string

const (
	execLogin execKind = "login"
	execRun   execKind = "run"
	execShell execKind = "shell"
)

// The exec commands own the terminal while they run (tea.Exec). Stubs.
type (
	loginExec struct {
		stdin          io.Reader
		stdout, stderr io.Writer
	}
	runExec struct {
		stdin          io.Reader
		stdout, stderr io.Writer
	}
	shellExec struct {
		stdin          io.Reader
		stdout, stderr io.Writer
	}
)

func (e *loginExec) Run() error            { return nil }
func (e *loginExec) SetStdin(r io.Reader)  { e.stdin = r }
func (e *loginExec) SetStdout(w io.Writer) { e.stdout = w }
func (e *loginExec) SetStderr(w io.Writer) { e.stderr = w }

func (e *runExec) Run() error            { return nil }
func (e *runExec) SetStdin(r io.Reader)  { e.stdin = r }
func (e *runExec) SetStdout(w io.Writer) { e.stdout = w }
func (e *runExec) SetStderr(w io.Writer) { e.stderr = w }

func (e *shellExec) Run() error            { return nil }
func (e *shellExec) SetStdin(r io.Reader)  { e.stdin = r }
func (e *shellExec) SetStdout(w io.Writer) { e.stdout = w }
func (e *shellExec) SetStderr(w io.Writer) { e.stderr = w }

// handleExecDone applies the outcome of a suspended subprocess. Stub.
func (m Model) handleExecDone(msg execDoneMsg) (Model, tea.Cmd) { return m, nil }

// beginLogin suspends the TUI for the login flow. Stub.
func (m Model) beginLogin(name string) (Model, tea.Cmd) { return m, nil }

// beginShell suspends the TUI for an isolated shell. Stub.
func (m Model) beginShell(name string) (Model, tea.Cmd) { return m, nil }

// newRunPrompt returns the run prompt modal for an account. Stub.
func newRunPrompt(name string) modal { return &runPrompt{account: name} }

type runPrompt struct{ account string }

func (p *runPrompt) update(m Model, msg tea.Msg) (Model, tea.Cmd) { return m, nil }
func (p *runPrompt) view(m Model) string                          { return "" }
