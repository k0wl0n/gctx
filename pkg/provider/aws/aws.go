// Package aws implements the provider contract on top of the AWS CLI v2.
//
// AWS has no global switch: an account is an AWS profile, selected per shell
// through AWS_PROFILE. gctx never writes credentials or tokens; SSO profiles
// are written with `aws configure set` and logged in with `aws sso login`.
package aws

import (
	"context"
	"encoding/json"
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
	cliName     = "aws"
	authSSO     = "sso"
	authProfile = "profile"
)

// ErrCLINotFound reports that the aws CLI is not on PATH.
var ErrCLINotFound = errors.New("aws CLI not found; install AWS CLI v2")

var profilePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// Options injects the AWS config location and process hooks.
type Options struct {
	ConfigFile string // "": $AWS_CONFIG_FILE, else ~/.aws/config
	LookPath   func(string) (string, error)
	Getenv     func(string) string
	Runner     provider.Runner // nil: runs the aws binary found by LookPath
}

// Provider is the AWS backend.
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

func (p *Provider) ID() provider.ID     { return provider.AWS }
func (p *Provider) Label() string       { return "AWS" }
func (p *Provider) TargetLabel() string { return "PROFILE" }

func (p *Provider) ValidateTarget(target string) error {
	if !profilePattern.MatchString(target) {
		return fmt.Errorf("invalid AWS profile name %q: use 1-64 letters, digits, '_', '.' or '-'", target)
	}
	return nil
}

// CLI looks aws up on PATH each time, so installing it is noticed without a restart.
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

func (p *Provider) configPath() string {
	if p.opts.ConfigFile != "" {
		return p.opts.ConfigFile
	}
	if path := p.opts.Getenv("AWS_CONFIG_FILE"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".aws", "config")
}

// childEnv points aws at the same config file gctx reads.
func (p *Provider) childEnv() []string {
	if path := p.configPath(); path != "" {
		return []string{"AWS_CONFIG_FILE=" + path}
	}
	return nil
}

// profiles returns each profile's keys from the AWS config file. A missing
// file yields no profiles.
func (p *Provider) profiles() map[string]map[string]string {
	profiles := map[string]map[string]string{}
	data, err := os.ReadFile(p.configPath())
	if err != nil {
		return profiles
	}
	var current map[string]string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
		case strings.HasPrefix(line, "["):
			current = nil
			header := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			name, isProfile := strings.CutPrefix(header, "profile ")
			if isProfile {
				name = strings.TrimSpace(name)
			} else if header == "default" {
				name = header
			} else {
				continue
			}
			current = map[string]string{}
			profiles[name] = current
		case current != nil:
			if key, value, ok := strings.Cut(line, "="); ok {
				current[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
	}
	return profiles
}

func usesSSO(keys map[string]string) bool {
	for key := range keys {
		if strings.HasPrefix(key, "sso_") {
			return true
		}
	}
	return false
}

// Create writes a new SSO profile with `aws configure set`, or adopts an
// existing profile. sso_account_id and sso_role_name are written only when supplied.
func (p *Provider) Create(ctx context.Context, acct config.Account) (provider.CreateOutcome, error) {
	profile := acct.Target()
	outcome := provider.CreateOutcome{Target: profile}
	if err := p.ValidateTarget(profile); err != nil {
		return outcome, err
	}
	_, exists := p.profiles()[profile]

	switch auth := acct.Settings["auth"]; auth {
	case "", authProfile:
		if !exists {
			return outcome, fmt.Errorf("AWS profile %q not found in %s; create it with `aws configure` or use auth=sso", profile, p.configPath())
		}
	case authSSO:
		if exists {
			return outcome, fmt.Errorf("AWS profile %q already exists in %s; use auth=profile to adopt it", profile, p.configPath())
		}
		if err := p.writeSSOProfile(ctx, profile, acct.Settings); err != nil {
			return outcome, err
		}
	default:
		return outcome, fmt.Errorf("unknown auth %q: use %q or %q", auth, authSSO, authProfile)
	}
	outcome.TargetSet = true
	return outcome, nil
}

func (p *Provider) writeSSOProfile(ctx context.Context, profile string, fields map[string]string) error {
	for _, key := range []string{"sso_start_url", "sso_region"} {
		if fields[key] == "" {
			return fmt.Errorf("auth=sso requires %s", key)
		}
	}
	runner, err := p.runner()
	if err != nil {
		return err
	}
	for _, key := range []string{"sso_start_url", "sso_region", "region", "sso_account_id", "sso_role_name"} {
		value := fields[key]
		if value == "" {
			continue
		}
		if _, err := runner.Query(ctx, p.childEnv(), "configure", "set", "profile."+profile+"."+key, value); err != nil {
			return fmt.Errorf("failed to write AWS profile %q: %w", profile, err)
		}
	}
	return nil
}

// Login runs `aws sso login` for the account's profile.
func (p *Provider) Login(ctx context.Context, acct config.Account, lio provider.LoginIO) (provider.LoginResult, error) {
	profile := acct.Target()
	result := provider.LoginResult{Account: acct.Name}
	keys, exists := p.profiles()[profile]
	if !exists {
		return result, fmt.Errorf("AWS profile %q not found in %s", profile, p.configPath())
	}
	if !usesSSO(keys) {
		return result, fmt.Errorf("profile %s does not use SSO; run aws configure", profile)
	}
	runner, err := p.runner()
	if err != nil {
		return result, err
	}

	args := []string{"sso", "login", "--profile", profile}
	if lio.NoBrowser {
		args = append(args, "--no-browser")
	}
	progress(lio, provider.LoginAuthStarted)
	stdio := provider.Stdio{Stdin: lio.Stdin, Stdout: lio.Stdout, Stderr: lio.Stderr}
	if err := runner.Interactive(ctx, stdio, p.childEnv(), args...); err != nil {
		return result, fmt.Errorf("aws sso login failed: %w", err)
	}
	progress(lio, provider.LoginAuthDone)
	result.Completed = []string{"sso login"}
	if warning, incomplete := incompleteProfileWarning(acct.Name, profile, p.profiles()[profile]); incomplete {
		result.Warnings = append(result.Warnings, warning)
	}
	return result, nil
}

// incompleteProfileWarning explains how to add the account id and role that
// STS checks need; gctx never reads the SSO token cache to discover them.
func incompleteProfileWarning(account, profile string, keys map[string]string) (provider.Warning, bool) {
	var missing, commands []string
	for _, row := range []struct{ key, placeholder string }{
		{"sso_account_id", "<id>"},
		{"sso_role_name", "<role>"},
	} {
		if keys[row.key] == "" {
			missing = append(missing, row.key)
			commands = append(commands, fmt.Sprintf("aws configure set profile.%s.%s %s", profile, row.key, row.placeholder))
		}
	}
	if len(missing) == 0 {
		return provider.Warning{}, false
	}
	return provider.Warning{
		Code:    provider.WarnProfileIncomplete,
		Account: account,
		Message: fmt.Sprintf("profile %s lacks %s; credential checks fail until set:\n%s", profile, strings.Join(missing, " and "), strings.Join(commands, "\n")),
	}, true
}

func progress(lio provider.LoginIO, kind provider.LoginEventKind) {
	if lio.Progress != nil {
		lio.Progress(provider.LoginEvent{Kind: kind})
	}
}

// Activate only hints at the shell command that applies the account: AWS has
// no global state to switch.
func (p *Provider) Activate(_ context.Context, acct config.Account) (provider.ActivateResult, error) {
	return provider.ActivateResult{
		Account:   acct.Name,
		ApplyHint: fmt.Sprintf(`eval "$(%s env %s)"`, config.AppName, acct.Name),
	}, nil
}

// Local checks that the profile exists in the AWS config file.
func (p *Provider) Local(acct config.Account) provider.LocalStatus {
	if _, exists := p.profiles()[acct.Target()]; !exists {
		return provider.LocalStatus{
			Status:   provider.HealthConfigMissing,
			Findings: []provider.Finding{{Health: provider.HealthConfigMissing}},
		}
	}
	return provider.LocalStatus{Status: provider.HealthReady}
}

// CheckAuth runs `aws sts get-caller-identity` for every account in parallel.
// Credentialed is keyed by account name.
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
			out, err := runner.Query(ctx, p.childEnv(), "sts", "get-caller-identity", "--profile", acct.Target(), "--output", "json")
			mu.Lock()
			defer mu.Unlock()
			if ctx.Err() != nil {
				report.Warnings = append(report.Warnings, authWarning(acct.Name, ctx.Err()))
				return
			}
			report.Credentialed[acct.Name] = err == nil
			if err != nil {
				return
			}
			var identity struct{ Arn string }
			if json.Unmarshal([]byte(out), &identity) == nil && identity.Arn != "" {
				report.Identities[acct.Name] = identity.Arn
			}
		}()
	}
	wg.Wait()
	return report
}

func authWarning(account string, err error) provider.Warning {
	return provider.Warning{Code: provider.WarnCredentialCheck, Account: account, Message: err.Error()}
}

func (p *Provider) Env(acct config.Account) ([]string, error) {
	return []string{"AWS_PROFILE=" + acct.Target()}, nil
}

// Delete leaves the user's AWS config untouched.
func (p *Provider) Delete(_ context.Context, acct config.Account, _ provider.DeleteOptions) (provider.DeleteResult, error) {
	return provider.DeleteResult{Account: acct.Name}, nil
}

func (p *Provider) Details(acct config.Account) []provider.Field {
	fields := []provider.Field{{Label: "Profile", Value: acct.Target()}}
	for _, row := range []struct{ label, key string }{
		{"Region", "region"},
		{"Auth", "auth"},
		{"SSO start URL", "sso_start_url"},
	} {
		if value := acct.Settings[row.key]; value != "" {
			fields = append(fields, provider.Field{Label: row.label, Value: value})
		}
	}
	return fields
}
