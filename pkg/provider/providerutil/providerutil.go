// Package providerutil holds helpers shared by CLI-backed providers.
package providerutil

import (
	"context"
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
