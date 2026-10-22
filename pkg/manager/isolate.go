package manager

import (
	"fmt"
	"os"
	"os/exec"

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
	return p.Env(account)
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
	env, err := p.Env(account)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(cli.Bin, args...)
	cmd.Env = append(os.Environ(), env...)
	return cmd, nil
}

// ShellCommand returns an unstarted shell for the account's isolated
// environment. The caller wires stdio and runs it.
func (m *Manager) ShellCommand(name string) (*exec.Cmd, error) {
	env, err := m.Env(name)
	if err != nil {
		return nil, err
	}
	shell := ResolveShell(m.opts.GOOS, m.opts.Getenv, m.opts.LookPath)
	cmd := exec.Command(shell)
	env = append(env, fmt.Sprintf("GCTX_ACCOUNT=%s", name))
	cmd.Env = append(cmd.Environ(), env...)
	return cmd, nil
}
