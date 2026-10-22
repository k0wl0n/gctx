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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/providerutil"
)

const (
	cliName     = "az"
	profileFile = "azureProfile.json"
	envConfDir  = "AZURE_CONFIG_DIR"
)

// ErrCLINotFound reports that the az CLI is not on PATH.
var ErrCLINotFound = errors.New("az CLI not found; install the Azure CLI")

var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// authFailureMarkers identify a token failure caused by a missing or expired
// login; any other failure leaves the account UNVERIFIED.
var authFailureMarkers = []string{
	"AADSTS", "az login", "Please run 'az login'", "Interactive authentication is needed",
	"expired", "refresh token", "No subscription found", "not logged in",
}

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
	return provider.CLIStatus{Name: cliName, Bin: bin, Found: err == nil}
}

func (p *Provider) runner() (provider.Runner, error) {
	return providerutil.ResolveRunner(p.opts.Runner, p.CLI(), ErrCLINotFound)
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

// Create makes the account's private config directory and returns it, with the
// tenant, as settings to store.
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
	outcome.Settings = map[string]string{"dir": dir}
	if tenant := acct.Settings["tenant"]; tenant != "" {
		outcome.Settings["tenant"] = tenant
	}
	outcome.TargetSet = true
	return outcome, nil
}

// Login steps, in the order they run.
const (
	stepIsolate = "isolate config"
	stepLogin   = "az login --use-device-code"
	stepVerify  = "verify az account get-access-token"
)

// deviceCodePage is shown when az names the code but not the page.
const deviceCodePage = "https://microsoft.com/devicelogin"

var (
	urlPattern = regexp.MustCompile(`https?://[^\s]+`)
	// Device codes are 8 to 12 upper-case letters or digits. Only the phrase
	// is case-insensitive, so prose such as "enter the code below" never matches.
	deviceCodePattern = regexp.MustCompile(`(?i:enter\s+the\s+code)\s+([A-Z0-9]{8,12})\b`)
	codeAlonePattern  = regexp.MustCompile(`^[A-Z0-9]{8,12}$`)
	codeLeadPattern   = regexp.MustCompile(`(?i)enter\s+the\s+code\s*$`)
)

// cleanOutputLine drops escape sequences and carriage returns from az output.
func cleanOutputLine(line string) string {
	return strings.TrimSpace(strings.ReplaceAll(ansi.Strip(line), "\r", ""))
}

// parseDeviceCode extracts the verification page and the code from az's
// "open the page ... and enter the code ABC123 to authenticate" message.
func parseDeviceCode(line string) (url, code string, ok bool) {
	line = cleanOutputLine(line)
	match := deviceCodePattern.FindStringSubmatch(line)
	if match == nil {
		return "", "", false
	}
	url = strings.TrimRight(urlPattern.FindString(line), ".,;)")
	if url == "" {
		url = deviceCodePage
	}
	return url, match[1], true
}

// deviceCodeScanner reads az output line by line. It also accepts a code on
// the line directly after one that ends with "enter the code".
type deviceCodeScanner struct {
	url           string // last page seen
	codeComesNext bool
}

func (s *deviceCodeScanner) scan(line string) (url, code string, ok bool) {
	line = cleanOutputLine(line)
	if found := strings.TrimRight(urlPattern.FindString(line), ".,;)"); found != "" {
		s.url = found
	}
	if url, code, ok := parseDeviceCode(line); ok {
		s.codeComesNext = false
		return url, code, true
	}
	expected := s.codeComesNext
	s.codeComesNext = codeLeadPattern.MatchString(line)
	if expected && codeAlonePattern.MatchString(line) {
		s.codeComesNext = false
		url := s.url
		if url == "" {
			url = deviceCodePage
		}
		return url, line, true
	}
	return "", "", false
}

func (p *Provider) StreamsLogin() bool { return true }

// loginReporter sends the streaming events when the caller listens.
type loginReporter struct{ progress func(provider.LoginEvent) }

func (r loginReporter) emit(event provider.LoginEvent) {
	if r.progress != nil {
		r.progress(event)
	}
}

func (r loginReporter) step(kind provider.LoginEventKind, step, detail string) {
	r.emit(provider.LoginEvent{Kind: kind, Step: step, Message: detail})
}

// Login runs `az login --use-device-code` in the isolated directory, selects
// the subscription there and checks a token can be obtained. With
// lio.Progress set, az runs with piped output: every line becomes a
// LoginOutput event, the device code a LoginDeviceCode event, and the lines
// are also written to lio.Stdout and lio.Stderr when those are set.
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
	report := loginReporter{progress: lio.Progress}
	setStep := "az account set --subscription " + acct.Target()
	for _, step := range []string{stepIsolate, stepLogin, setStep, stepVerify} {
		report.step(provider.LoginStepPending, step, "")
	}

	report.step(provider.LoginStepStarted, stepIsolate, "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		report.step(provider.LoginStepFailed, stepIsolate, err.Error())
		return result, fmt.Errorf("failed to create azure config dir: %w", err)
	}
	report.step(provider.LoginStepDone, stepIsolate, envConfDir+"="+dir)
	env := []string{envConfDir + "=" + dir}

	args := []string{"login", "--use-device-code"}
	if tenant := acct.Settings["tenant"]; tenant != "" {
		args = append(args, "--tenant", tenant)
	}
	providerutil.Progress(lio, provider.LoginAuthStarted)
	report.step(provider.LoginStepStarted, stepLogin, "")
	if err := p.runLogin(ctx, runner, env, args, lio, report); err != nil {
		report.step(provider.LoginStepFailed, stepLogin, err.Error())
		return result, fmt.Errorf("az login failed: %w", err)
	}
	report.step(provider.LoginStepDone, stepLogin, "")
	result.Completed = []string{"az login"}

	report.step(provider.LoginStepStarted, setStep, "")
	if _, err := runner.Query(ctx, env, "account", "set", "--subscription", acct.Target()); err != nil {
		report.step(provider.LoginStepFailed, setStep, err.Error())
		return result, fmt.Errorf("az account set failed: %w", err)
	}
	report.step(provider.LoginStepDone, setStep, "")
	result.Completed = append(result.Completed, "account set")

	report.step(provider.LoginStepStarted, stepVerify, "")
	if _, err := runner.Query(ctx, env, "account", "get-access-token", "--subscription", acct.Target(), "--query", "expiresOn", "--output", "tsv"); err != nil {
		report.step(provider.LoginStepFailed, stepVerify, err.Error())
		return result, fmt.Errorf("az login could not be verified: %w", err)
	}
	report.step(provider.LoginStepDone, stepVerify, "")
	providerutil.Progress(lio, provider.LoginAuthDone)
	result.Completed = append(result.Completed, "verified")
	return result, nil
}

// runLogin runs az login interactively, or streamed when the caller listens
// and the runner can stream.
func (p *Provider) runLogin(ctx context.Context, runner provider.Runner, env, args []string, lio provider.LoginIO, report loginReporter) error {
	streamer, canStream := runner.(provider.StreamingRunner)
	if lio.Progress == nil || !canStream {
		stdio := provider.Stdio{Stdin: lio.Stdin, Stdout: lio.Stdout, Stderr: lio.Stderr}
		return runner.Interactive(ctx, stdio, env, args...)
	}
	report.emit(provider.LoginEvent{Kind: provider.LoginOutput, Output: "$ " + cliName + " " + strings.Join(args, " ")})
	var scanner deviceCodeScanner
	codeReported := false
	return streamer.Stream(ctx, env, func(stream, line string) {
		report.emit(provider.LoginEvent{Kind: provider.LoginOutput, Output: line})
		if !codeReported {
			if url, code, ok := scanner.scan(line); ok {
				codeReported = true
				report.emit(provider.LoginEvent{Kind: provider.LoginDeviceCode, URL: url, Code: code})
			}
		}
		out := lio.Stdout
		if stream == provider.StreamStderr {
			out = lio.Stderr
		}
		if out != nil {
			fmt.Fprintln(out, line)
		}
	}, args...)
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
// of the output) for every account in parallel. Results are keyed by
// account name; a failure is not logged in only when az says so (authFailureMarkers).
func (p *Provider) CheckAuth(ctx context.Context, accts []config.Account) provider.AuthReport {
	runner, err := p.runner()
	if err != nil {
		return provider.AuthReport{
			Results:  map[string]provider.AuthResult{},
			Warnings: []provider.Warning{providerutil.CredentialWarning("", err)},
		}
	}
	return providerutil.CheckEach(ctx, accts, func(ctx context.Context, acct config.Account) provider.AuthResult {
		dir, err := p.accountDir(acct)
		if err != nil {
			return provider.AuthResult{}
		}
		out, err := runner.Query(ctx, []string{envConfDir + "=" + dir},
			"account", "get-access-token", "--subscription", acct.Target(), "--query", "expiresOn", "--output", "tsv")
		if err != nil {
			return providerutil.FailedCheck(err, authFailureMarkers)
		}
		return provider.AuthResult{Credentialed: out != ""}
	})
}

func (p *Provider) Env(acct config.Account) ([]string, error) {
	dir, err := p.accountDir(acct)
	if err != nil {
		return nil, err
	}
	return []string{envConfDir + "=" + dir}, nil
}

// Delete runs `az logout` in the isolated directory when asked, then removes
// the directory when asked and it is inside the root. A failed logout is a
// warning: the user still wants the account gone.
func (p *Provider) Delete(ctx context.Context, acct config.Account, opt provider.DeleteOptions) (provider.DeleteResult, error) {
	result := provider.DeleteResult{Account: acct.Name}
	dir, err := p.accountDir(acct)
	if err != nil {
		return result, err
	}
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return result, nil
	}
	if opt.Logout {
		if warning := p.logout(ctx, acct.Name, dir); warning != nil {
			result.Warnings = append(result.Warnings, *warning)
		}
	}
	if !opt.RemoveDir {
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

func (p *Provider) logout(ctx context.Context, account, dir string) *provider.Warning {
	runner, err := p.runner()
	if err == nil {
		_, err = runner.Query(ctx, []string{envConfDir + "=" + dir}, "logout")
	}
	if err == nil {
		return nil
	}
	return &provider.Warning{Code: provider.WarnLogoutFailed, Account: account, Message: "az logout failed: " + err.Error()}
}

var _ provider.DeleteDescriber = (*Provider)(nil)

// DeleteItems offers the isolated directory (on by default, with its size) and
// a server-side logout (off by default).
func (p *Provider) DeleteItems(acct config.Account) []provider.DeleteItem {
	dir, err := p.accountDir(acct)
	if err != nil {
		return nil
	}
	if _, err := os.Lstat(dir); err != nil {
		return nil
	}
	return []provider.DeleteItem{
		{Key: provider.DeleteKeyRemoveDir, Label: "isolated az dir " + dir, Detail: formatSize(dirSize(dir)) + ", cached tokens", Default: true},
		{Key: provider.DeleteKeyLogout, Label: "run az logout first", Detail: "revokes the refresh token server-side"},
	}
}

func (p *Provider) Kept(config.Account) string {
	return "the Azure subscription and its resources. " + config.AppName + " never touches cloud resources."
}

// dirSize adds up regular file sizes below dir without following symlinks.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value, suffix := float64(bytes)/unit, "KB"
	for _, next := range []string{"MB", "GB"} {
		if value < unit {
			break
		}
		value, suffix = value/unit, next
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}
