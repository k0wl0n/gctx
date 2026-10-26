package manager

import (
	"fmt"
	"os"
	"os/exec"
	"slices"

	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/provider"
)

// ResolveShell picks the shell for `gctx shell`: $SHELL when set (and, on
// Windows, resolvable), otherwise the platform default.
func ResolveShell(goos string, getenv func(string) string, lookPath func(string) (string, error)) string {
	shell := getenv("SHELL")
	if goos != "windows" {
		if shell == "" {
			return "/bin/sh"
		}
		return shell
	}
	if shell != "" {
		if resolved, err := lookPath(shell); err == nil {
			return resolved
		}
	}
	for _, candidate := range []string{"pwsh.exe", "powershell.exe"} {
		if resolved, err := lookPath(candidate); err == nil {
			return resolved
		}
	}
	if comspec := getenv("COMSPEC"); comspec != "" {
		return comspec
	}
	return "cmd.exe"
}

// Env returns the environment that runs a process as the account without
// touching global state.
func (m *Manager) Env(name string) ([]string, error) {
	account, p, err := m.resolve(name)
	if err != nil {
		return nil, err
	}
	return m.accountEnv(p, account)
}

// RunCommand returns an unstarted `<provider CLI> <args>` command that runs as
// the account without touching global state. The caller wires stdio and runs it.
func (m *Manager) RunCommand(name string, args []string) (*exec.Cmd, error) {
	account, p, err := m.resolve(name)
	if err != nil {
		return nil, err
	}
	cli := p.CLI()
	if !cli.Found {
		if p.ID() == provider.GCP {
			return nil, gcloud.ErrNotFound
		}
		return nil, fmt.Errorf("%s CLI not found on PATH", p.Label())
	}
	env, err := m.accountEnv(p, account)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(cli.Bin, args...)
	cmd.Env = provider.MergeEnv(os.Environ(), env)
	return cmd, nil
}

// ShellScope selects which accounts' environments a subshell gets.
type ShellScope int

const (
	// ShellAllActive merges the environment of every slot's active account
	// with the selected account; the selected account wins its own slot.
	ShellAllActive ShellScope = iota
	// ShellOnly gives the selected account's environment alone.
	ShellOnly
)

// ShellCommand returns an unstarted shell for the account's isolated
// environment. The caller wires stdio and runs it.
func (m *Manager) ShellCommand(name string, scope ShellScope) (*exec.Cmd, error) {
	selected, selectedProvider, err := m.resolve(name)
	if err != nil {
		return nil, err
	}
	var env []string
	if scope == ShellAllActive {
		if env, err = m.activeSlotsEnv(selectedProvider.Slot()); err != nil {
			return nil, err
		}
	}
	own, err := m.accountEnv(selectedProvider, selected)
	if err != nil {
		return nil, err
	}
	env = append(env, own...)
	shell := ResolveShell(m.opts.GOOS, m.opts.Getenv, m.opts.LookPath)
	cmd := exec.Command(shell)
	env = append(env, fmt.Sprintf("GCTX_ACCOUNT=%s", name))
	cmd.Env = provider.MergeEnv(cmd.Environ(), env)
	return cmd, nil
}

// activeSlotsEnv is the environment of each slot's active account except
// skipSlot, in slot-name order. Active accounts that no longer exist are
// skipped.
func (m *Manager) activeSlotsEnv(skipSlot string) ([]string, error) {
	cfg, err := m.loadConfig()
	if err != nil {
		return nil, err
	}
	slots := make([]string, 0, len(cfg.ActiveBySlot))
	for slot := range cfg.ActiveBySlot {
		slots = append(slots, slot)
	}
	slices.Sort(slots)
	var env []string
	for _, slot := range slots {
		if slot == skipSlot || cfg.ActiveBySlot[slot] == "" {
			continue
		}
		account, p, err := m.resolve(cfg.ActiveBySlot[slot])
		if err != nil {
			continue
		}
		accountEnv, err := m.accountEnv(p, account)
		if err != nil {
			return nil, err
		}
		env = append(env, accountEnv...)
	}
	return env, nil
}
