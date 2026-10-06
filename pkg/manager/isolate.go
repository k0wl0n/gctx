package manager

import (
	"fmt"
	"os/exec"

	"github.com/k0wl0n/gctx/pkg/gcloud"
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

func isolationEnv(configName, adcPath string) []string {
	return []string{
		"CLOUDSDK_ACTIVE_CONFIG_NAME=" + configName,
		"GOOGLE_APPLICATION_CREDENTIALS=" + adcPath,
	}
}

// RunCommand returns an unstarted `gcloud <args>` command that runs as the
// account without touching global state. The caller wires stdio and runs it.
func (m *Manager) RunCommand(name string, args []string) (*exec.Cmd, error) {
	account, err := m.account(name)
	if err != nil {
		return nil, err
	}
	m.locateGcloud()
	client := m.client()
	if !client.Available() {
		return nil, gcloud.ErrNotFound
	}
	return client.Command(isolationEnv(account.ConfigName, account.ADCPath), args...), nil
}

// ShellCommand returns an unstarted shell for the account's isolated
// environment. The caller wires stdio and runs it.
func (m *Manager) ShellCommand(name string) (*exec.Cmd, error) {
	account, err := m.account(name)
	if err != nil {
		return nil, err
	}
	shell := ResolveShell(m.opts.GOOS, m.opts.Getenv, m.opts.LookPath)
	cmd := exec.Command(shell)
	env := append(isolationEnv(account.ConfigName, account.ADCPath), fmt.Sprintf("GCTX_ACCOUNT=%s", name))
	cmd.Env = append(cmd.Environ(), env...)
	return cmd, nil
}
