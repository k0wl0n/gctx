package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/present"
)

type execKind string

const (
	execLogin execKind = "login"
	execRun   execKind = "run"
	execShell execKind = "shell"
)

// The exec commands own the terminal while they run (tea.Exec).
type (
	loginExec struct {
		stdin          io.Reader
		stdout, stderr io.Writer
		svc            Service
		name           string
		configName     string
		ctx            context.Context
	}
	runExec struct {
		stdin          io.Reader
		stdout, stderr io.Writer
		svc            Service
		name           string
		configName     string
		args           []string
	}
	shellExec struct {
		stdin          io.Reader
		stdout, stderr io.Writer
		svc            Service
		name           string
	}
)

func (e *loginExec) SetStdin(r io.Reader)  { e.stdin = r }
func (e *loginExec) SetStdout(w io.Writer) { e.stdout = w }
func (e *loginExec) SetStderr(w io.Writer) { e.stderr = w }

func (e *loginExec) Run() error {
	fmt.Fprintf(e.stdout, "Logging in to %s (%s)…\n", e.name, e.configName)
	lio := present.LoginIO(e.stdin, e.stdout, e.stderr)
	ctx := e.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := e.svc.Login(ctx, e.name, lio)
	present.LoginDone(e.stdout, e.stderr, result, err)
	if err != nil {
		fmt.Fprintf(e.stdout, "\nPress Enter to return to gctx")
		waitEnter(e.stdin)
		// Return a wrapped loginResult so handleExecDone can inspect it.
		return &loginError{result: result, err: err}
	}
	// Store result in error for propagation; nil means success.
	return &loginError{result: result, err: nil}
}

// loginError carries the LoginResult through the exec callback.
type loginError struct {
	result manager.LoginResult
	err    error
}

func (l *loginError) Error() string {
	if l.err != nil {
		return l.err.Error()
	}
	return ""
}
func (l *loginError) Unwrap() error { return l.err }

func (e *runExec) SetStdin(r io.Reader)  { e.stdin = r }
func (e *runExec) SetStdout(w io.Writer) { e.stdout = w }
func (e *runExec) SetStderr(w io.Writer) { e.stderr = w }

func (e *runExec) Run() error {
	argsStr := strings.Join(e.args, " ")
	fmt.Fprintf(e.stdout, "── gctx: running as %s: gcloud %s ──\n", e.name, argsStr)
	cmd, err := e.svc.RunCommand(e.name, e.args)
	exitCode := 0
	if err != nil {
		fmt.Fprintf(e.stderr, "error: %v\n", err)
		exitCode = 1
	} else {
		cmd.Stdin = e.stdin
		cmd.Stdout = e.stdout
		cmd.Stderr = e.stderr
		if runErr := cmd.Run(); runErr != nil {
			var exitErr interface{ ExitCode() int }
			if errors.As(runErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
	}
	fmt.Fprintf(e.stdout, "── exit status %d · press Enter to return to gctx ──", exitCode)
	waitEnter(e.stdin)
	return &runError{exitCode: exitCode}
}

// runError carries the exit code through the exec callback.
type runError struct{ exitCode int }

func (r *runError) Error() string { return fmt.Sprintf("exit %d", r.exitCode) }

func (e *shellExec) SetStdin(r io.Reader)  { e.stdin = r }
func (e *shellExec) SetStdout(w io.Writer) { e.stdout = w }
func (e *shellExec) SetStderr(w io.Writer) { e.stderr = w }

func (e *shellExec) Run() error {
	fmt.Fprintf(e.stdout, "Starting shell for account '%s'... Type 'exit' to return to gctx.\n", e.name)
	cmd, err := e.svc.ShellCommand(e.name)
	if err != nil {
		return err
	}
	cmd.Stdin = e.stdin
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr
	exitCode := 0
	if runErr := cmd.Run(); runErr != nil {
		var exitErr interface{ ExitCode() int }
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return &shellError{exitCode: exitCode}
}

// shellError carries the shell exit code through the exec callback.
type shellError struct{ exitCode int }

func (s *shellError) Error() string { return fmt.Sprintf("exit %d", s.exitCode) }

// handleExecDone applies the outcome of a suspended subprocess.
func (m Model) handleExecDone(msg execDoneMsg) (Model, tea.Cmd) {
	m.reload()
	switch msg.kind {
	case execLogin:
		return m.handleLoginDone(msg)
	case execRun:
		return m.handleRunDone(msg)
	case execShell:
		return m.handleShellDone(msg)
	}
	return m, nil
}

func (m Model) handleLoginDone(msg execDoneMsg) (Model, tea.Cmd) {
	var le *loginError
	if errors.As(msg.err, &le) {
		if le.err == nil {
			// Success.
			cmd := m.startCredsCheck()
			return m, tea.Batch(m.setStatus("Login complete for "+msg.account, levelInfo), cmd)
		}
		// Determine which step failed.
		var stepErr *gcloud.StepError
		var statusText string
		if errors.As(le.err, &stepErr) && stepErr.Step == manager.StepADCLogin {
			statusText = fmt.Sprintf("gcloud login succeeded but ADC was not saved for %s — press l to retry", msg.account)
		} else {
			statusText = fmt.Sprintf("Login cancelled for %s; credentials unchanged", msg.account)
		}
		cmd := m.startCredsCheck()
		return m, tea.Batch(m.setStatus(statusText, levelError), cmd)
	}
	// Unexpected error (e.g. subprocess could not start).
	cmd := m.startCredsCheck()
	return m, tea.Batch(m.setStatus("Login failed: "+msg.err.Error(), levelError), cmd)
}

func (m Model) handleRunDone(msg execDoneMsg) (Model, tea.Cmd) {
	var re *runError
	if errors.As(msg.err, &re) {
		if m.debugLog != nil {
			m.debugLog.Info("run", "account", msg.account, "exit", re.exitCode)
		}
		if re.exitCode == 0 {
			return m, m.setStatus("gcloud exited with status 0", levelInfo)
		}
		return m, m.setStatus(fmt.Sprintf("gcloud exited with status %d", re.exitCode), levelError)
	}
	return m, nil
}

func (m Model) handleShellDone(msg execDoneMsg) (Model, tea.Cmd) {
	var se *shellError
	if errors.As(msg.err, &se) {
		text := fmt.Sprintf("Shell for %s exited", msg.account)
		if se.exitCode != 0 {
			text += fmt.Sprintf(" (status %d)", se.exitCode)
		}
		return m, m.setStatus(text, levelInfo)
	}
	if msg.err != nil {
		return m, m.setStatus("Shell error: "+msg.err.Error(), levelError)
	}
	return m, m.setStatus("Shell for "+msg.account+" exited", levelInfo)
}

// beginLogin suspends the TUI for the login flow.
func (m Model) beginLogin(name string) (Model, tea.Cmd) {
	account, ok := m.selectedAccount()
	configName := name + "-config"
	if ok {
		configName = account.ConfigName
	}
	exec := &loginExec{svc: m.svc, name: name, configName: configName, ctx: context.Background()}
	cmd := m.exec(exec, func(err error) tea.Msg {
		return execDoneMsg{kind: execLogin, account: name, err: err}
	})
	return m, cmd
}

// beginShell suspends the TUI for an isolated shell.
func (m Model) beginShell(name string) (Model, tea.Cmd) {
	exec := &shellExec{svc: m.svc, name: name}
	cmd := m.exec(exec, func(err error) tea.Msg {
		return execDoneMsg{kind: execShell, account: name, err: err}
	})
	return m, cmd
}

// newRunPrompt returns the run prompt modal for an account.
func newRunPrompt(name string) modal { return &runPrompt{account: name} }

// runPrompt is the modal shown when the user presses 'r'.
type runPrompt struct {
	account    string
	input      string
	history    []string // borrowed from model; points into m.history slice
	historyPos int      // -1 = fresh input
	draft      string   // saved draft when browsing history
	errMsg     string
}

func (p *runPrompt) update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.modal = nil
		return m, nil
	case "enter":
		trimmed := strings.TrimSpace(p.input)
		if trimmed == "" {
			return m, nil
		}
		goos := m.goos
		if goos == "" {
			goos = "linux"
		}
		args, err := tokenize(trimmed, m.svc.GcloudBin(), goos)
		if err != nil {
			p.errMsg = err.Error()
			return m, nil
		}
		if len(args) == 0 {
			return m, nil
		}
		// Add to history.
		m.history = addHistory(m.history, trimmed)
		m.modal = nil
		// Launch run exec.
		account := p.account
		re := &runExec{svc: m.svc, name: account, args: args}
		cmd := m.exec(re, func(err error) tea.Msg {
			return execDoneMsg{kind: execRun, account: account, err: err}
		})
		return m, cmd
	case "up":
		history := m.history
		if len(history) == 0 {
			return m, nil
		}
		if p.historyPos < 0 {
			p.draft = p.input
			p.historyPos = len(history) - 1
		} else if p.historyPos > 0 {
			p.historyPos--
		}
		p.input = history[p.historyPos]
		p.errMsg = ""
		return m, nil
	case "down":
		if p.historyPos < 0 {
			return m, nil
		}
		if p.historyPos < len(m.history)-1 {
			p.historyPos++
			p.input = m.history[p.historyPos]
		} else {
			p.historyPos = -1
			p.input = p.draft
		}
		p.errMsg = ""
		return m, nil
	case "backspace":
		if len(p.input) > 0 {
			runes := []rune(p.input)
			p.input = string(runes[:len(runes)-1])
			p.errMsg = ""
		}
		return m, nil
	default:
		if t := key.Text; t != "" {
			p.input += t
			p.errMsg = ""
			p.historyPos = -1
		}
	}
	return m, nil
}

func (p *runPrompt) view(m Model) string {
	account, ok := m.overview.Account(p.account)
	var hint string
	if ok && !account.ADCExists {
		hint = "⚠ No saved ADC: client libraries will not find credentials\n"
	}
	prompt := fmt.Sprintf("Run as: %s\ngcloud > %s", p.account, p.input)
	if p.errMsg != "" {
		prompt += "\n" + p.errMsg
	}
	return hint + prompt
}

// addHistory appends entry to history, capping at historyLimit.
func addHistory(history []string, entry string) []string {
	// Avoid consecutive duplicates.
	if len(history) > 0 && history[len(history)-1] == entry {
		return history
	}
	history = append(history, entry)
	if len(history) > historyLimit {
		history = history[len(history)-historyLimit:]
	}
	return history
}

// waitEnter reads from r until a newline or EOF.
func waitEnter(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
}
