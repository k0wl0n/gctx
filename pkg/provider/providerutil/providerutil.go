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

// CheckEach runs check for every account concurrently. Credentialed and
// Identities are keyed by account name; an account whose check was cut short by
// ctx gets a warning instead of a result. An empty identity is not recorded.
func CheckEach(ctx context.Context, accts []config.Account, check func(ctx context.Context, acct config.Account) (credentialed bool, identity string)) provider.AuthReport {
	report := provider.AuthReport{
		Credentialed: map[string]bool{},
		Identities:   map[string]string{},
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, acct := range accts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			credentialed, identity := check(ctx, acct)
			mu.Lock()
			defer mu.Unlock()
			if ctx.Err() != nil {
				report.Warnings = append(report.Warnings, CredentialWarning(acct.Name, ctx.Err()))
				return
			}
			report.Credentialed[acct.Name] = credentialed
			if identity != "" {
				report.Identities[acct.Name] = identity
			}
		}()
	}
	wg.Wait()
	return report
}
