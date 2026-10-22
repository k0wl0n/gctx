// Package azure implements the provider contract on top of the Azure CLI.
//
// Azure has no global switch: each account owns an isolated AZURE_CONFIG_DIR
// under the configured root, selected per shell through `gctx env`. gctx never
// reads Azure token caches and never asks az for an access token.
package azure

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

const (
	cliName     = "az"
	profileFile = "azureProfile.json"
	envConfDir  = "AZURE_CONFIG_DIR"
)

// ErrCLINotFound reports that the az CLI is not on PATH.
var ErrCLINotFound = errors.New("az CLI not found; install the Azure CLI")

var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Options injects the isolated-directory root and process hooks.
type Options struct {
	ConfigRoot string // parent of every account's AZURE_CONFIG_DIR
	LookPath   func(string) (string, error)
	Runner     provider.Runner // nil: runs the az binary found by LookPath
	Getenv     func(string) string
}

// Provider is the Azure backend.
type Provider struct {
	opts Options
}

var _ provider.Provider = (*Provider)(nil)

func New(opts Options) *Provider {
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	return &Provider{opts: opts}
}

func (p *Provider) ID() provider.ID     { return provider.Azure }
func (p *Provider) Label() string       { return "Azure" }
func (p *Provider) TargetLabel() string { return "SUBSCRIPTION" }

func (p *Provider) ValidateTarget(target string) error {
	if !guidPattern.MatchString(target) {
		return fmt.Errorf("invalid Azure subscription id %q: expected a GUID such as 00000000-0000-0000-0000-000000000000", target)
	}
	return nil
}

// CLI looks az up on PATH each time, so installing it is noticed without a restart.
func (p *Provider) CLI() provider.CLIStatus {
	bin, err := p.opts.LookPath(cliName)
	return provider.CLIStatus{Bin: bin, Found: err == nil}
}

func (p *Provider) runner() (provider.Runner, error) {
	if p.opts.Runner != nil {
		return p.opts.Runner, nil
	}
	cli := p.CLI()
	if !cli.Found {
		return nil, ErrCLINotFound
	}
	return execrunner.New(cli.Bin), nil
}

// accountDir returns the account's isolated directory, refusing any path that
// is not inside the root (a hand-edited config must not redirect file operations).
func (p *Provider) accountDir(acct config.Account) (string, error) {
	dir := acct.Settings["dir"]
	if dir == "" {
		if err := validateName(acct.Name); err != nil {
			return "", err
		}
		dir = filepath.Join(p.opts.ConfigRoot, acct.Name)
	}
	if !p.insideRoot(dir) {
		return "", fmt.Errorf("azure config dir %q is outside %s", dir, p.opts.ConfigRoot)
	}
	return dir, nil
}

// insideRoot reports whether dir, after cleaning and resolving symlinks, is a
// strict descendant of the root.
func (p *Provider) insideRoot(dir string) bool {
	if p.opts.ConfigRoot == "" || dir == "" {
		return false
	}
	root, target := resolve(p.opts.ConfigRoot), resolve(dir)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolve cleans path and resolves symlinks of its longest existing prefix.
func resolve(path string) string {
	path = filepath.Clean(path)
	var missing []string
	for current := path; ; current = filepath.Dir(current) {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved
		}
		if filepath.Dir(current) == current {
			return path
		}
		missing = append(missing, filepath.Base(current))
	}
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || filepath.Base(name) != name {
		return fmt.Errorf("account name %q cannot be used as an Azure config directory", name)
	}
	return nil
}

// Create makes the account's private config directory and records it in
// acct.Settings["dir"].
func (p *Provider) Create(_ context.Context, acct config.Account) (provider.CreateOutcome, error) {
	outcome := provider.CreateOutcome{Target: acct.Target()}
	if err := p.ValidateTarget(acct.Target()); err != nil {
		return outcome, err
	}
	if err := validateName(acct.Name); err != nil {
		return outcome, err
	}
	dir := filepath.Join(p.opts.ConfigRoot, acct.Name)
	if !p.insideRoot(dir) {
		return outcome, fmt.Errorf("azure config dir %q is outside %s", dir, p.opts.ConfigRoot)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return outcome, fmt.Errorf("failed to create azure config dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return outcome, fmt.Errorf("failed to restrict azure config dir: %w", err)
	}
	if acct.Settings != nil {
		acct.Settings["dir"] = dir
	}
	outcome.TargetSet = true
	return outcome, nil
}

// Login runs `az login --use-device-code` in the isolated directory, then
// selects the subscription there.
func (p *Provider) Login(ctx context.Context, acct config.Account, lio provider.LoginIO) (provider.LoginResult, error) {
	result := provider.LoginResult{Account: acct.Name}
	dir, err := p.accountDir(acct)
	if err != nil {
		return result, err
	}
	runner, err := p.runner()
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return result, fmt.Errorf("failed to create azure config dir: %w", err)
	}
	env := []string{envConfDir + "=" + dir}

	args := []string{"login", "--use-device-code"}
	if tenant := acct.Settings["tenant"]; tenant != "" {
		args = append(args, "--tenant", tenant)
	}
	progress(lio, provider.LoginAuthStarted)
	stdio := provider.Stdio{Stdin: lio.Stdin, Stdout: lio.Stdout, Stderr: lio.Stderr}
	if err := runner.Interactive(ctx, stdio, env, args...); err != nil {
		return result, fmt.Errorf("az login failed: %w", err)
	}
	result.Completed = []string{"az login"}
	if _, err := runner.Query(ctx, env, "account", "set", "--subscription", acct.Target()); err != nil {
		return result, fmt.Errorf("az account set failed: %w", err)
	}
	progress(lio, provider.LoginAuthDone)
	result.Completed = append(result.Completed, "account set")
	return result, nil
}

func progress(lio provider.LoginIO, kind provider.LoginEventKind) {
	if lio.Progress != nil {
		lio.Progress(provider.LoginEvent{Kind: kind})
	}
}

// Activate only hints at the shell command that applies the account: Azure
// has no global state to switch.
func (p *Provider) Activate(_ context.Context, acct config.Account) (provider.ActivateResult, error) {
	return provider.ActivateResult{
		Account:   acct.Name,
		ApplyHint: fmt.Sprintf(`eval "$(%s env %s)"`, config.AppName, acct.Name),
	}, nil
}

// Local stats the config directory and profile file; token caches are never read.
func (p *Provider) Local(acct config.Account) provider.LocalStatus {
	dir, err := p.accountDir(acct)
	if err != nil {
		return provider.LocalStatus{
			Status:   provider.HealthError,
			Findings: []provider.Finding{{Health: provider.HealthError, Message: err.Error()}},
		}
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return provider.LocalStatus{
			Status:   provider.HealthConfigMissing,
			Findings: []provider.Finding{{Health: provider.HealthConfigMissing}},
		}
	}
	if _, err := os.Stat(filepath.Join(dir, profileFile)); err != nil {
		return provider.LocalStatus{
			Status:   provider.HealthAuthRequired,
			Findings: []provider.Finding{{Health: provider.HealthAuthRequired}},
		}
	}
	return provider.LocalStatus{Status: provider.HealthReady}
}

// CheckAuth asks az only for the token's expiry (--query keeps the token out
// of the output) for every account in parallel. Credentialed is keyed by
// account name; a non-zero exit means expired or not logged in.
func (p *Provider) CheckAuth(ctx context.Context, accts []config.Account) provider.AuthReport {
	report := provider.AuthReport{
		Credentialed: map[string]bool{},
		Identities:   map[string]string{},
	}
	runner, err := p.runner()
	if err != nil {
		report.Warnings = append(report.Warnings, authWarning("", err))
		return report
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, acct := range accts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			credentialed := p.checkOne(ctx, runner, acct)
			mu.Lock()
			defer mu.Unlock()
			if ctx.Err() != nil {
				report.Warnings = append(report.Warnings, authWarning(acct.Name, ctx.Err()))
				return
			}
			report.Credentialed[acct.Name] = credentialed
		}()
	}
	wg.Wait()
	return report
}

func (p *Provider) checkOne(ctx context.Context, runner provider.Runner, acct config.Account) bool {
	dir, err := p.accountDir(acct)
	if err != nil {
		return false
	}
	out, err := runner.Query(ctx, []string{envConfDir + "=" + dir},
		"account", "get-access-token", "--subscription", acct.Target(), "--query", "expiresOn", "--output", "tsv")
	return err == nil && out != ""
}

func authWarning(account string, err error) provider.Warning {
	return provider.Warning{Code: provider.WarnCredentialCheck, Account: account, Message: err.Error()}
}

func (p *Provider) Env(acct config.Account) ([]string, error) {
	dir, err := p.accountDir(acct)
	if err != nil {
		return nil, err
	}
	return []string{envConfDir + "=" + dir}, nil
}

// Delete removes the isolated directory when it is inside the root. Logging
// out of Azure is left to the user.
func (p *Provider) Delete(_ context.Context, acct config.Account, _ provider.DeleteOptions) (provider.DeleteResult, error) {
	result := provider.DeleteResult{Account: acct.Name}
	dir, err := p.accountDir(acct)
	if err != nil {
		return result, err
	}
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return result, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return result, fmt.Errorf("failed to remove azure config dir: %w", err)
	}
	result.Removed = []string{dir}
	return result, nil
}

func (p *Provider) Details(acct config.Account) []provider.Field {
	fields := []provider.Field{{Label: "Subscription", Value: acct.Target()}}
	if tenant := acct.Settings["tenant"]; tenant != "" {
		fields = append(fields, provider.Field{Label: "Tenant", Value: tenant})
	}
	if dir, err := p.accountDir(acct); err == nil {
		fields = append(fields, provider.Field{Label: "Config dir", Value: dir})
	}
	return fields
}
