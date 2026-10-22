// Package gcp implements the provider contract on top of the gcloud CLI.
package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/k0wl0n/gctx/pkg/adc"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/gcloud"
	"github.com/k0wl0n/gctx/pkg/gcloudconfig"
	"github.com/k0wl0n/gctx/pkg/provider"
)

const credentialCheckTimeout = 15 * time.Second

// ErrInvalidProjectID reports a malformed GCP project ID.
var ErrInvalidProjectID = errors.New("invalid project ID")

var (
	projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	projectDomain    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*\.[a-z]{2,}$`)
)

// ValidateProjectID checks a GCP project ID, including the domain-scoped
// "<domain>:<id>" form.
func ValidateProjectID(projectID string) error {
	id := projectID
	if domain, rest, scoped := strings.Cut(projectID, ":"); scoped {
		if !projectDomain.MatchString(domain) {
			return invalidProject(projectID)
		}
		id = rest
	}
	if !projectIDPattern.MatchString(id) {
		return invalidProject(projectID)
	}
	return nil
}

func invalidProject(projectID string) error {
	return fmt.Errorf("%w %q: project IDs are 6-30 characters: lowercase letters, digits, hyphens; start with a letter; no trailing hyphen", ErrInvalidProjectID, projectID)
}

// ConfigNameFor returns the gcloud configuration name used for an account.
func ConfigNameFor(accountName string) string { return accountName + "-config" }

// Options injects the gcloud locations and process hooks.
type Options struct {
	GcloudDir  string
	ADCDir     string
	DefaultADC string
	Client     *gcloud.Client // nil: located on PATH by Locate
	LookPath   func(string) (string, error)
	Getenv     func(string) string
	Runner     provider.Runner // nil: runs the current Client
}

// Provider is the GCP backend. It never reads or writes config.json.
type Provider struct {
	opts  Options
	store adc.Store
	auto  bool

	mu     sync.Mutex
	client *gcloud.Client
}

var (
	_ provider.Provider          = (*Provider)(nil)
	_ provider.StatusInspector   = (*Provider)(nil)
	_ provider.CredentialSaver   = (*Provider)(nil)
	_ provider.AuthLister        = (*Provider)(nil)
	_ provider.ConfigChecker     = (*Provider)(nil)
	_ provider.GlobalStateReader = (*Provider)(nil)
)

func New(opts Options) *Provider {
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	p := &Provider{
		opts:   opts,
		store:  adc.Store{Dir: opts.ADCDir, DefaultPath: opts.DefaultADC},
		client: opts.Client,
	}
	if p.client == nil {
		p.auto = true
		p.Locate()
	}
	return p
}

// Locate looks gcloud up on PATH again when the client was not injected.
func (p *Provider) Locate() {
	if !p.auto {
		return
	}
	client := &gcloud.Client{}
	if bin, err := p.opts.LookPath("gcloud"); err == nil {
		client.Bin = bin
	}
	p.mu.Lock()
	p.client = client
	p.mu.Unlock()
}

// Client returns the current gcloud client.
func (p *Provider) Client() *gcloud.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client
}

func (p *Provider) runner() provider.Runner {
	if p.opts.Runner != nil {
		return p.opts.Runner
	}
	return NewRunner(p.Client())
}

func (p *Provider) requireGcloud() error {
	p.Locate()
	if !p.Client().Available() {
		return gcloud.ErrNotFound
	}
	return nil
}

func (p *Provider) ID() provider.ID                    { return provider.GCP }
func (p *Provider) Label() string                      { return "GCP" }
func (p *Provider) TargetLabel() string                { return "PROJECT" }
func (p *Provider) ValidateTarget(target string) error { return ValidateProjectID(target) }

// CLI re-locates gcloud first, so installing it is noticed without a restart.
func (p *Provider) CLI() provider.CLIStatus {
	p.Locate()
	client := p.Client()
	return provider.CLIStatus{Bin: client.Bin, Found: client.Available()}
}

func accountEnv(configName string) []string {
	return []string{"CLOUDSDK_ACTIVE_CONFIG_NAME=" + configName}
}

func isolationEnv(configName, adcPath string) []string {
	return []string{
		"CLOUDSDK_ACTIVE_CONFIG_NAME=" + configName,
		"GOOGLE_APPLICATION_CREDENTIALS=" + adcPath,
	}
}

func setProjectWarning(account string, err error) provider.Warning {
	return provider.Warning{Code: provider.WarnSetProjectFailed, Account: account, Message: detailed(err)}
}

// detailed renders an error with gcloud's output appended, as the legacy
// "%w\n%s" messages did.
func detailed(err error) string {
	var stepErr *gcloud.StepError
	if errors.As(err, &stepErr) && stepErr.Output != "" {
		return fmt.Sprintf("%v\n%s", err, stepErr.Output)
	}
	return err.Error()
}

func stepOutput(err error) string {
	var stepErr *gcloud.StepError
	if errors.As(err, &stepErr) {
		return stepErr.Output
	}
	return ""
}

func progress(lio provider.LoginIO, kind provider.LoginEventKind, message string) {
	if lio.Progress != nil {
		lio.Progress(provider.LoginEvent{Kind: kind, Message: message})
	}
}

// ConfigExists reports whether a gcloud configuration file exists.
func (p *Provider) ConfigExists(configName string) bool {
	existing, _ := gcloudconfig.Read(p.opts.GcloudDir, configName)
	return existing.Exists
}

// Create creates and activates the account's gcloud configuration and tries
// to set its project. A set-project failure is a warning: it is expected
// before the account is authenticated.
func (p *Provider) Create(ctx context.Context, acct config.Account) (provider.CreateOutcome, error) {
	configName := acct.ConfigName
	if configName == "" {
		configName = ConfigNameFor(acct.Name)
	}
	outcome := provider.CreateOutcome{ConfigName: configName, Target: acct.ProjectID}
	if err := p.requireGcloud(); err != nil {
		return outcome, err
	}
	runner := p.runner()
	if _, err := runner.Query(ctx, nil, "config", "configurations", "create", configName); err != nil {
		if output := stepOutput(err); !strings.Contains(output, "already exists") {
			return outcome, fmt.Errorf("failed to create config: %w\n%s", err, output)
		}
	}
	if _, err := runner.Query(ctx, nil, "config", "configurations", "activate", configName); err != nil {
		return outcome, err
	}
	if _, err := runner.Query(ctx, accountEnv(configName), "config", "set", "project", acct.ProjectID); err != nil {
		outcome.Warnings = append(outcome.Warnings, setProjectWarning(acct.Name, err))
	} else {
		outcome.TargetSet = true
	}
	return outcome, nil
}

// Login runs the interactive gcloud login and ADC login, then stores the
// resulting ADC. The result reports how far it got on failure.
func (p *Provider) Login(ctx context.Context, acct config.Account, lio provider.LoginIO) (provider.LoginResult, error) {
	result := provider.LoginResult{Account: acct.Name}
	if err := p.requireGcloud(); err != nil {
		return result, err
	}
	runner := p.runner()
	scoped := runner.WithEnv(accountEnv(acct.ConfigName))
	stdio := provider.Stdio{Stdin: lio.Stdin, Stdout: lio.Stdout, Stderr: lio.Stderr}

	if _, err := runner.Query(ctx, nil, "config", "configurations", "activate", acct.ConfigName); err != nil {
		return result, fmt.Errorf("failed to activate gcloud config: %w", err)
	}

	authArgs := []string{"auth", "login"}
	adcArgs := []string{"auth", "application-default", "login"}
	if lio.NoBrowser {
		authArgs = append(authArgs, "--no-browser")
		adcArgs = append(adcArgs, "--no-browser")
	}

	progress(lio, provider.LoginAuthStarted, "")
	if err := scoped.Interactive(ctx, stdio, nil, authArgs...); err != nil {
		return result, fmt.Errorf("auth login failed: %w", err)
	}
	result.GcloudLoggedIn = true
	progress(lio, provider.LoginAuthDone, "")

	progress(lio, provider.LoginADCStarted, "")
	before := adc.Snapshot(p.opts.DefaultADC)
	if err := scoped.Interactive(ctx, stdio, nil, adcArgs...); err != nil {
		return result, fmt.Errorf("ADC auth failed: %w", err)
	}
	refreshed := before.Changed(adc.Snapshot(p.opts.DefaultADC))

	if acct.ProjectID != "" {
		if _, err := scoped.Query(ctx, nil, "config", "set", "project", acct.ProjectID); err != nil {
			warning := setProjectWarning(acct.Name, err)
			result.Warnings = append(result.Warnings, warning)
			progress(lio, provider.LoginSetProjectFailed, warning.Message)
		}
	}

	progress(lio, provider.LoginVerifyingADC, "")
	adcPath, err := p.store.Save(acct.Name)
	if err != nil {
		return result, err
	}
	if !refreshed {
		result.Warnings = append(result.Warnings, provider.Warning{
			Code: provider.WarnADCNotRefreshed, Account: acct.Name,
			Message: "gcloud did not rewrite the default ADC file; the existing file was saved",
		})
	}
	result.ADCSaved, result.ADCPath = true, adcPath
	result.Email = p.accountEmail(ctx, acct.ConfigName)
	return result, nil
}

// accountEmail reads core/account from the account's own configuration.
func (p *Provider) accountEmail(ctx context.Context, configName string) string {
	email, err := p.runner().Query(ctx, accountEnv(configName), "config", "get-value", "account")
	if err != nil {
		return ""
	}
	return email
}

// SaveCredentials copies the default ADC into the account's stored location
// and reads the account's email when gcloud is installed.
func (p *Provider) SaveCredentials(ctx context.Context, acct config.Account) (adcPath, email string, err error) {
	if adcPath, err = p.store.Save(acct.Name); err != nil {
		return "", "", err
	}
	p.Locate()
	if p.Client().Available() {
		email = p.accountEmail(ctx, acct.ConfigName)
	}
	return adcPath, email, nil
}

// Activate restores the account's ADC, activates its gcloud configuration and
// re-applies its project. A re-authentication failure is reported as a
// WarnReauthRequired warning.
func (p *Provider) Activate(ctx context.Context, acct config.Account) (provider.ActivateResult, error) {
	result := provider.ActivateResult{Account: acct.Name}

	if _, statErr := os.Stat(p.store.StoragePath(acct.Name)); statErr != nil {
		result.Warnings = append(result.Warnings, provider.Warning{
			Code: provider.WarnADCMissing, Account: acct.Name,
			Message: fmt.Sprintf("no saved ADC for account: %s", acct.Name),
		})
	} else if err := p.store.Restore(acct.Name); err != nil {
		return result, err
	}

	if err := p.requireGcloud(); err != nil {
		return result, err
	}
	runner := p.runner()
	if _, err := runner.Query(ctx, nil, "config", "configurations", "activate", acct.ConfigName); err != nil {
		return result, err
	}
	if _, err := runner.Query(ctx, accountEnv(acct.ConfigName), "config", "set", "project", acct.ProjectID); err != nil {
		warning := setProjectWarning(acct.Name, err)
		if strings.Contains(warning.Message, "Reauthentication required") {
			warning.Code = provider.WarnReauthRequired
		}
		result.Warnings = append(result.Warnings, warning)
	}
	return result, nil
}

// Status is the file-based view of a GCP account, with the GCP-only details
// the manager reports alongside the shared LocalStatus.
type Status = provider.InspectedStatus

// Inspect reads the account's stored ADC and gcloud configuration. Session
// credential state is not consulted: AUTH REQUIRED is reported here only when
// the configuration names no account.
func (p *Provider) Inspect(acct config.Account) Status {
	var status Status
	if acct.ADCPath != "" {
		if info, err := os.Stat(acct.ADCPath); err == nil {
			status.RecordedADCModTime = info.ModTime()
		}
	}

	var errorFindings []provider.Finding
	for _, candidate := range []string{p.store.StoragePath(acct.Name), acct.ADCPath} {
		if candidate == "" || (candidate == acct.ADCPath && !p.store.Contains(candidate)) {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil {
			if !os.IsNotExist(err) {
				errorFindings = append(errorFindings, provider.Finding{Health: provider.HealthError, Message: "cannot read saved ADC file: " + err.Error()})
			}
			continue
		}
		if info.IsDir() {
			continue
		}
		status.StoredADCPath, status.ADCExists, status.ADCModTime = candidate, true, info.ModTime()
		if err := adc.Validate(candidate); err != nil {
			errorFindings = append(errorFindings, provider.Finding{Health: provider.HealthError, Message: "saved ADC file is invalid: " + err.Error()})
		}
		break
	}

	gcfg, err := gcloudconfig.Read(p.opts.GcloudDir, acct.ConfigName)
	if err != nil {
		errorFindings = append(errorFindings, provider.Finding{Health: provider.HealthError, Message: err.Error()})
	}
	status.ConfigExists = gcfg.Exists
	status.ConfigAccount, status.ConfigProject = gcfg.Account, gcfg.Project

	local := &status.Local
	local.Identity = gcfg.Account
	local.Findings = append(local.Findings, errorFindings...)
	if err == nil && !gcfg.Exists {
		local.Findings = append(local.Findings, provider.Finding{Health: provider.HealthConfigMissing})
	}
	if gcfg.Exists && gcfg.Account == "" {
		local.Findings = append(local.Findings, provider.Finding{Health: provider.HealthAuthRequired})
	}
	if !status.ADCExists && len(errorFindings) == 0 {
		local.Findings = append(local.Findings, provider.Finding{Health: provider.HealthADCMissing})
	}
	if gcfg.Project != "" && gcfg.Project != acct.ProjectID {
		local.Findings = append(local.Findings, provider.Finding{Health: provider.HealthProjectMismatch})
	}
	local.Status = provider.HealthReady
	if len(local.Findings) > 0 {
		local.Status = local.Findings[0].Health
	}
	return status
}

func (p *Provider) Local(acct config.Account) provider.LocalStatus { return p.Inspect(acct).Local }

// GlobalState reads the active gcloud configuration and compares the default
// ADC with storedADCPath in memory; no content or hash is kept.
func (p *Provider) GlobalState(storedADCPath string) provider.GlobalState {
	state := provider.GlobalState{
		ActiveConfig:          gcloudconfig.ActiveConfig(p.opts.GcloudDir),
		DefaultCredentialPath: p.opts.DefaultADC,
	}
	info, err := os.Stat(p.opts.DefaultADC)
	if err != nil || info.IsDir() {
		return state
	}
	state.DefaultCredentialExists, state.DefaultCredentialModTime = true, info.ModTime()
	if storedADCPath == "" {
		return state
	}
	if equal, err := adc.FilesEqual(p.opts.DefaultADC, storedADCPath); err == nil {
		state.Compared, state.Matches = true, equal
	}
	return state
}

// ListAuth runs `gcloud auth list` without network access and without
// printing or refreshing any token. It returns every listed account and the
// report; Credentialed is keyed by lower-cased gcloud account.
func (p *Provider) ListAuth(ctx context.Context) ([]string, provider.AuthReport) {
	p.Locate()
	if !p.Client().Available() {
		return nil, provider.AuthReport{Warnings: []provider.Warning{{Code: provider.WarnGcloudNotFound, Message: gcloud.ErrNotFound.Error()}}}
	}
	out, err := p.runner().Query(ctx, nil, "auth", "list", "--format=json")
	if err != nil {
		return nil, provider.AuthReport{Warnings: []provider.Warning{{Code: provider.WarnCredentialCheck, Message: err.Error()}}}
	}
	var entries []struct {
		Account string `json:"account"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, provider.AuthReport{Warnings: []provider.Warning{{Code: provider.WarnCredentialCheck, Message: fmt.Sprintf("unexpected gcloud auth list output: %v", err)}}}
	}
	var accounts []string
	credentialed := map[string]bool{}
	for _, entry := range entries {
		accounts = append(accounts, entry.Account)
		// ACTIVE and CREDENTIALED both mean gcloud holds credentials on file.
		if strings.EqualFold(entry.Status, "ACTIVE") || strings.EqualFold(entry.Status, "CREDENTIALED") {
			credentialed[strings.ToLower(entry.Account)] = true
		}
	}
	return accounts, provider.AuthReport{Credentialed: credentialed}
}

func (p *Provider) CheckAuth(ctx context.Context, _ []config.Account) provider.AuthReport {
	_, report := p.ListAuth(ctx)
	return report
}

func (p *Provider) Env(acct config.Account) ([]string, error) {
	return isolationEnv(acct.ConfigName, acct.ADCPath), nil
}

// Delete removes the stored ADC and, when asked, the gcloud configuration.
// Failures are warnings; Removed lists the configuration name when deleted.
func (p *Provider) Delete(ctx context.Context, acct config.Account, opt provider.DeleteOptions) (provider.DeleteResult, error) {
	result := provider.DeleteResult{Account: acct.Name}

	outside, err := p.store.Remove(acct.Name, acct.ADCPath)
	if outside != "" {
		result.Warnings = append(result.Warnings, provider.Warning{
			Code: provider.WarnADCPathOutsideStore, Account: acct.Name,
			Message: fmt.Sprintf("recorded ADC path %s is outside the %s ADC directory and was not deleted", outside, config.AppName),
		})
	}
	if err != nil {
		result.Warnings = append(result.Warnings, provider.Warning{Code: provider.WarnADCRemoveFailed, Account: acct.Name, Message: err.Error()})
	}

	if opt.GcloudConfig {
		p.Locate()
		if _, err := p.runner().Query(ctx, nil, "config", "configurations", "delete", acct.ConfigName, "--quiet"); err != nil {
			message := err.Error()
			var stepErr *gcloud.StepError
			if errors.As(err, &stepErr) && stepErr.LastLine() != "" {
				message = stepErr.LastLine()
			}
			result.Warnings = append(result.Warnings, provider.Warning{Code: provider.WarnConfigDeleteFailed, Account: acct.Name, Message: message})
		} else {
			result.Removed = append(result.Removed, acct.ConfigName)
		}
	}
	return result, nil
}

func (p *Provider) Details(acct config.Account) []provider.Field {
	return []provider.Field{
		{Label: "gcloud config", Value: acct.ConfigName},
		{Label: "ADC path", Value: acct.ADCPath},
	}
}

// clientRunner adapts *gcloud.Client to provider.Runner and picks the
// per-command timeout gcloud needs.
type clientRunner struct {
	client *gcloud.Client
	extra  []string
}

// NewRunner returns a Runner over the given gcloud client.
func NewRunner(client *gcloud.Client) provider.Runner { return clientRunner{client: client} }

func (r clientRunner) env(env []string) []string {
	return append(append([]string(nil), r.extra...), env...)
}

func (r clientRunner) Query(ctx context.Context, env []string, args ...string) (string, error) {
	return r.client.Query(ctx, timeoutFor(args), r.env(env), args...)
}

func (r clientRunner) Interactive(ctx context.Context, stdio provider.Stdio, env []string, args ...string) error {
	return r.client.Interactive(ctx, gcloud.Stdio{Stdin: stdio.Stdin, Stdout: stdio.Stdout, Stderr: stdio.Stderr}, r.env(env), args...)
}

func (r clientRunner) WithEnv(extra []string) provider.Runner {
	return clientRunner{client: r.client, extra: r.env(extra)}
}

func timeoutFor(args []string) time.Duration {
	command := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(command, "auth list"):
		return credentialCheckTimeout
	case strings.HasPrefix(command, "config set "):
		return gcloud.SetProjectTimeout
	default:
		return gcloud.DefaultTimeout
	}
}
