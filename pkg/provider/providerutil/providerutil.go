// Package providerutil holds helpers shared by CLI-backed providers.
package providerutil

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

// ResolveRunner returns injected when set, else an execrunner for the CLI
// described by cli, or notFound when the CLI is not installed.
func ResolveRunner(injected provider.Runner, cli provider.CLIStatus, notFound error) (provider.Runner, error) {
	if injected != nil {
		return injected, nil
	}
	if !cli.Found {
		return nil, notFound
	}
	return execrunner.New(cli.Bin), nil
}

// Progress reports a login progress point when the caller listens.
func Progress(lio provider.LoginIO, kind provider.LoginEventKind) {
	if lio.Progress != nil {
		lio.Progress(provider.LoginEvent{Kind: kind})
	}
}

// CredentialWarning describes a credential check that could not complete.
func CredentialWarning(account string, err error) provider.Warning {
	return provider.Warning{Code: provider.WarnCredentialCheck, Account: account, Message: err.Error()}
}

// CheckEach runs check for every account concurrently. Results are keyed by
// account name; an account whose check was cut short by ctx gets a warning and
// a result carrying only ctx's error.
func CheckEach(ctx context.Context, accts []config.Account, check func(ctx context.Context, acct config.Account) provider.AuthResult) provider.AuthReport {
	report := provider.AuthReport{Results: map[string]provider.AuthResult{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, acct := range accts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := check(ctx, acct)
			mu.Lock()
			defer mu.Unlock()
			if err := ctx.Err(); err != nil {
				report.Warnings = append(report.Warnings, CredentialWarning(acct.Name, err))
				result = provider.AuthResult{Err: err}
			}
			report.Results[acct.Name] = result
		}()
	}
	wg.Wait()
	return report
}

// IsAuthFailure reports whether CLI output contains one of the markers that
// identify a rejected or expired credential (case-insensitive).
func IsAuthFailure(output string, markers []string) bool {
	output = strings.ToLower(output)
	for _, marker := range markers {
		if strings.Contains(output, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// FailedCheck turns a failed credential check into a result: a known auth
// failure is a not-credentialed verdict; anything else (network, timeout,
// unknown) carries err and becomes UNVERIFIED.
func FailedCheck(err error, markers []string) provider.AuthResult {
	output := err.Error()
	var commandErr *provider.CommandError
	if errors.As(err, &commandErr) {
		output = commandErr.Output
	}
	if IsAuthFailure(output, markers) {
		return provider.AuthResult{}
	}
	return provider.AuthResult{Err: err}
}
