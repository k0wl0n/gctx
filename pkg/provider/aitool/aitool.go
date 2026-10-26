// Package aitool holds the behaviour shared by AI coding-tool providers
// (Claude Code, Codex): each account owns a private config directory the tool
// is pointed at through one environment variable, selected per shell with
// `gctx env`. The tool's own files in that directory (tokens, history,
// settings) are never opened; health comes from the tool's status command.
package aitool

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/providerutil"
)

// Spec describes one tool.
type Spec struct {
	ID          provider.ID
	Label       string
	TargetName  string // detail label of the target, e.g. "Org"
	CLIName     string
	EnvVar      string   // variable that points the tool at its config dir
	DefaultDir  string   // directory under $HOME used when EnvVar is unset
	LoginArgs   []string // interactive login, run with EnvVar set
	InstallHint string
	NotFound    error
}

// Options injects the directory roots and process hooks.
type Options struct {
	ConfigRoot string // parent of every account's isolated directory
	Home       string // user home, for discovering an existing setup
	LookPath   func(string) (string, error)
	Runner     provider.Runner // nil: runs the binary found by LookPath
	Getenv     func(string) string
	// Processes lists the command names of running processes; nil runs ps.
	Processes func(ctx context.Context) ([]string, error)
}

// Tool implements the provider methods every AI tool shares.
type Tool struct {
	spec Spec
	opts Options
}

func NewTool(spec Spec, opts Options) *Tool {
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.Processes == nil {
		opts.Processes = psCommands
	}
	return &Tool{spec: spec, opts: opts}
}

func (t *Tool) ID() provider.ID             { return t.spec.ID }
func (t *Tool) Label() string               { return t.spec.Label }
func (t *Tool) Category() provider.Category { return provider.CategoryAITool }
func (t *Tool) Slot() string                { return string(t.spec.ID) }
func (t *Tool) EnvBased() bool              { return true }
func (t *Tool) ChecksOverNetwork() bool     { return true }
func (t *Tool) InstallHint() string         { return t.spec.InstallHint }
func (t *Tool) Columns() []provider.Column  { return nil }
func (t *Tool) Kept(config.Account) string {
	return t.spec.Label + " subscriptions and projects. " + config.AppName + " never touches them."
}
func (t *Tool) CLI() provider.CLIStatus { return t.cliStatus() }
func (t *Tool) Runner() (provider.Runner, error) {
	return providerutil.ResolveRunner(t.opts.Runner, t.cliStatus(), t.spec.NotFound)
}

func (t *Tool) cliStatus() provider.CLIStatus {
	bin, err := t.opts.LookPath(t.spec.CLIName)
	return provider.CLIStatus{Name: t.spec.CLIName, Bin: bin, Found: err == nil}
}

// ValidateTarget accepts any single-line label: the target is the org or
// workspace name, shown only.
func (t *Tool) ValidateTarget(target string) error {
	if strings.IndexFunc(target, unicode.IsControl) >= 0 {
		return fmt.Errorf("label %q must be a single line of text", target)
	}
	return nil
}

// Adopted reports whether the account's directory existed before gctx (the
// first-run import): gctx never removes or moves it.
func Adopted(acct config.Account) bool {
	return acct.Settings[provider.SettingConfigOrigin] == provider.ConfigOriginImported
}

// AccountDir is settings.dir, else <root>/<name>.
func (t *Tool) AccountDir(acct config.Account) (string, error) {
	if dir := acct.Settings[provider.SettingDir]; dir != "" {
		if !filepath.IsAbs(dir) {
			return "", fmt.Errorf("%s config dir %q is not an absolute path", t.spec.Label, dir)
		}
		return filepath.Clean(dir), nil
	}
	if err := validateName(acct.Name); err != nil {
		return "", err
	}
	if t.opts.ConfigRoot == "" {
		return "", errors.New("no config root for " + t.spec.Label + " accounts")
	}
	return filepath.Join(t.opts.ConfigRoot, acct.Name), nil
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || filepath.Base(name) != name {
		return fmt.Errorf("account name %q cannot be used as a config directory", name)
	}
	return nil
}

// ensureDir creates dir private to the user; an existing directory keeps its mode.
func ensureDir(dir string) error {
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// Create makes the account's private directory and records it.
func (t *Tool) Create(_ context.Context, acct config.Account) (provider.CreateOutcome, error) {
	outcome := provider.CreateOutcome{Target: acct.Target()}
	if err := t.ValidateTarget(acct.Target()); err != nil {
		return outcome, err
	}
	dir, err := t.AccountDir(acct)
	if err != nil {
		return outcome, err
	}
	if err := ensureDir(dir); err != nil {
		return outcome, fmt.Errorf("failed to create %s config dir: %w", t.spec.Label, err)
	}
	outcome.Settings = map[string]string{provider.SettingDir: dir}
	outcome.TargetSet = true
	return outcome, nil
}

// Login runs the tool's own login attached to the terminal, inside the
// account's directory (created first: some tools refuse a missing one).
func (t *Tool) Login(ctx context.Context, acct config.Account, lio provider.LoginIO) (provider.LoginResult, error) {
	result := provider.LoginResult{Account: acct.Name}
	if via := acct.Settings[provider.SettingVia]; via != "" {
		return result, fmt.Errorf("%s runs on %s: log in that account instead", acct.Name, via)
	}
	dir, err := t.AccountDir(acct)
	if err != nil {
		return result, err
	}
	runner, err := t.Runner()
	if err != nil {
		return result, err
	}
	if err := ensureDir(dir); err != nil {
		return result, fmt.Errorf("failed to create %s config dir: %w", t.spec.Label, err)
	}
	providerutil.Progress(lio, provider.LoginAuthStarted)
	stdio := provider.Stdio{Stdin: lio.Stdin, Stdout: lio.Stdout, Stderr: lio.Stderr}
	if err := runner.Interactive(ctx, stdio, t.dirEnv(dir), t.spec.LoginArgs...); err != nil {
		return result, fmt.Errorf("%s %s failed: %w", t.spec.CLIName, strings.Join(t.spec.LoginArgs, " "), err)
	}
	providerutil.Progress(lio, provider.LoginAuthDone)
	result.Completed = []string{t.spec.CLIName + " " + strings.Join(t.spec.LoginArgs, " ")}
	return result, nil
}

// Activate hints at the env line and counts the tool's running processes,
// which keep the account they started with.
func (t *Tool) Activate(ctx context.Context, acct config.Account) (provider.ActivateResult, error) {
	return provider.ActivateResult{
		Account:          acct.Name,
		ApplyHint:        fmt.Sprintf(`eval "$(%s env %s)"`, config.AppName, acct.Name),
		RunningProcesses: t.runningProcesses(ctx),
	}, nil
}

func (t *Tool) runningProcesses(ctx context.Context) int {
	commands, err := t.opts.Processes(ctx)
	if err != nil {
		return 0
	}
	count := 0
	for _, command := range commands {
		base := filepath.Base(strings.TrimSpace(command))
		if base == t.spec.CLIName || base == t.spec.CLIName+".exe" {
			count++
		}
	}
	return count
}

// psCommands lists running command names with ps; unsupported on Windows.
func psCommands(ctx context.Context) ([]string, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.New("process listing is not supported on windows")
	}
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "comm=").Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\n"), nil
}

// Local stats the directory only: login state is known from CheckAuth.
func (t *Tool) Local(acct config.Account) provider.LocalStatus {
	dir, err := t.AccountDir(acct)
	if err != nil {
		return provider.LocalStatus{
			Status:   provider.HealthError,
			Findings: []provider.Finding{{Health: provider.HealthError, Message: err.Error()}},
		}
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return provider.LocalStatus{Status: provider.HealthAuthRequired, Findings: []provider.Finding{{Health: provider.HealthAuthRequired}}}
	}
	return provider.LocalStatus{Status: provider.HealthReady}
}

// Env points the tool at the account's directory.
func (t *Tool) Env(acct config.Account) ([]string, error) {
	dir, err := t.AccountDir(acct)
	if err != nil {
		return nil, err
	}
	return t.dirEnv(dir), nil
}

// dirEnv points the tool at dir. The tool's own default directory is the one
// place it must not be pointed at: Claude Code keeps its login in the macOS
// keychain only while the variable is unset, so the variable is removed
// instead (a bare name, see provider.MergeEnv).
func (t *Tool) dirEnv(dir string) []string {
	if t.isDefaultDir(dir) {
		return []string{t.spec.EnvVar}
	}
	return []string{t.spec.EnvVar + "=" + dir}
}

func (t *Tool) isDefaultDir(dir string) bool {
	return t.opts.Home != "" && filepath.Clean(dir) == filepath.Join(t.opts.Home, t.spec.DefaultDir)
}

// CheckEach runs status for every account that has its own login and an
// existing directory; a missing directory is not logged in. Accounts that
// run on a linked account are left out: their health is the linked one's.
func (t *Tool) CheckEach(ctx context.Context, accts []config.Account, status func(ctx context.Context, runner provider.Runner, env []string) provider.AuthResult) provider.AuthReport {
	runner, err := t.Runner()
	if err != nil {
		return provider.AuthReport{
			Results:  map[string]provider.AuthResult{},
			Warnings: []provider.Warning{providerutil.CredentialWarning("", err)},
		}
	}
	var own []config.Account
	for _, acct := range accts {
		if acct.Settings[provider.SettingVia] == "" {
			own = append(own, acct)
		}
	}
	return providerutil.CheckEach(ctx, own, func(ctx context.Context, acct config.Account) provider.AuthResult {
		dir, err := t.AccountDir(acct)
		if err != nil {
			return provider.AuthResult{}
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return provider.AuthResult{}
		}
		return status(ctx, runner, t.dirEnv(dir))
	})
}

// Details lists the directory and, for adopted setups, that it is kept.
func (t *Tool) Details(acct config.Account) []provider.Field {
	var fields []provider.Field
	if label := acct.Target(); label != "" {
		fields = append(fields, provider.Field{Label: t.spec.TargetName, Value: label})
	}
	if dir, err := t.AccountDir(acct); err == nil {
		value := dir
		if Adopted(acct) {
			value += " (existing setup, never removed)"
		}
		fields = append(fields, provider.Field{Label: "Config dir", Value: value})
	}
	return fields
}

// ownDir reports whether Delete may remove dir: it was created by gctx as a
// direct child of the root and no other account records it.
func (t *Tool) ownDir(acct config.Account, dir string, opt provider.DeleteOptions) bool {
	if Adopted(acct) || opt.Keep[t.dirClaim(dir)] || t.opts.ConfigRoot == "" {
		return false
	}
	return filepath.Dir(resolve(dir)) == resolve(t.opts.ConfigRoot)
}

// Delete removes the directory when asked and gctx owns it. The tool's
// server-side session is left alone: the tools have no non-interactive logout
// gctx relies on.
func (t *Tool) Delete(_ context.Context, acct config.Account, opt provider.DeleteOptions) (provider.DeleteResult, error) {
	result := provider.DeleteResult{Account: acct.Name}
	dir, err := t.AccountDir(acct)
	if err != nil {
		return result, err
	}
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return result, nil
	}
	if !opt.RemoveDir || !t.ownDir(acct, dir, opt) {
		return result, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return result, fmt.Errorf("failed to remove %s config dir: %w", t.spec.Label, err)
	}
	result.Removed = []string{dir}
	return result, nil
}

// DeleteItems offers the isolated directory; an adopted one is shown locked off.
func (t *Tool) DeleteItems(acct config.Account) []provider.DeleteItem {
	dir, err := t.AccountDir(acct)
	if err != nil {
		return nil
	}
	if _, err := os.Lstat(dir); err != nil {
		return nil
	}
	if !t.ownDir(acct, dir, provider.DeleteOptions{}) {
		return []provider.DeleteItem{{Key: provider.DeleteKeyRemoveDir, Label: "config dir " + dir, Detail: "existing setup, kept", Locked: true}}
	}
	return []provider.DeleteItem{{Key: provider.DeleteKeyRemoveDir, Label: "isolated " + t.spec.CLIName + " dir " + dir, Detail: "login, settings and history", Default: true}}
}

func (t *Tool) dirClaim(dir string) string {
	return t.spec.Label + " config dir " + filepath.Clean(dir)
}

// Claims lists the account's directory, so two accounts never share one.
func (t *Tool) Claims(acct config.Account) []string {
	dir, err := t.AccountDir(acct)
	if err != nil {
		return nil
	}
	return []string{t.dirClaim(dir)}
}

// Rename moves a directory gctx created under the account's name; any other
// directory (adopted, or named otherwise) stays where it is.
func (t *Tool) Rename(_ context.Context, acct config.Account, newName string) (config.Account, error) {
	from, err := t.AccountDir(acct)
	if err != nil {
		return acct, err
	}
	if Adopted(acct) || t.opts.ConfigRoot == "" || resolve(from) != resolve(filepath.Join(t.opts.ConfigRoot, acct.Name)) {
		acct.Name = newName
		return acct, nil
	}
	if err := validateName(newName); err != nil {
		return acct, err
	}
	to := filepath.Join(t.opts.ConfigRoot, newName)
	if _, err := os.Lstat(to); err == nil {
		return acct, fmt.Errorf("%s config dir %s already exists", t.spec.Label, to)
	}
	if _, err := os.Lstat(from); err == nil {
		if err := os.Rename(from, to); err != nil {
			return acct, fmt.Errorf("failed to move %s config dir: %w", t.spec.Label, err)
		}
	}
	acct.Settings = maps.Clone(acct.Settings)
	if acct.Settings == nil {
		acct.Settings = map[string]string{}
	}
	acct.Settings[provider.SettingDir] = to
	acct.Name = newName
	return acct, nil
}

// DefaultDir is $EnvVar, else ~/<DefaultDir>; "" without either.
func (t *Tool) DefaultDir() string {
	if dir := t.opts.Getenv(t.spec.EnvVar); dir != "" {
		return dir
	}
	if t.opts.Home == "" {
		return ""
	}
	return filepath.Join(t.opts.Home, t.spec.DefaultDir)
}

// DiscoverSetups offers the tool's existing default directory as one account
// that keeps using it in place. It only stats the directory.
func (t *Tool) DiscoverSetups() (string, []provider.DiscoveredSetup, error) {
	dir := t.DefaultDir()
	if dir == "" {
		return "", nil, nil
	}
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return dir, nil, nil
	}
	if err != nil {
		return dir, nil, err
	}
	if !info.IsDir() {
		return dir, nil, nil
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return dir, []provider.DiscoveredSetup{{
		Name: t.spec.CLIName + "-default",
		Settings: map[string]string{
			provider.SettingDir:          dir,
			provider.SettingConfigOrigin: provider.ConfigOriginImported,
		},
		Note: "existing setup, used in place",
	}}, nil
}

// Footer implements provider.FormFooter.
func (t *Tool) Footer(values map[string]string) (writes, never string) {
	dir := filepath.Join(t.opts.ConfigRoot, "<name>")
	if name := values[provider.FormNameKey]; name != "" {
		dir = filepath.Join(t.opts.ConfigRoot, name)
	}
	return "config.json (name, org label) · isolated directory " + dir, "tokens, history or the tool's own settings"
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
