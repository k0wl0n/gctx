// Package secretref handles references to secrets; gctx stores a reference,
// never the secret itself.
//
// A resolved value is returned only to the caller: it is never logged and
// never part of an error, including the output of a failing helper.
package secretref

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"unicode"

	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

// Reference schemes.
const (
	SchemeOnePassword = "op"
	SchemeGCPSecret   = "gcpsm"
	SchemeKeychain    = "keychain"
	SchemeEnv         = "env"
)

const (
	errRawKey    = "store a reference, not the key"
	latestSecret = "latest"
)

var envVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Ref is a parsed secret reference. Path is everything after the scheme
// marker ("op://", "gcpsm://", "keychain:", "env:").
type Ref struct{ Scheme, Path string }

// Parse accepts op://vault/item/field, gcpsm://project/secret[/version],
// keychain:<service> and env:<VAR>. Errors never echo the input.
func Parse(s string) (Ref, error) {
	if LooksLikeKey(s) {
		return Ref{}, errors.New(errRawKey)
	}
	if strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return Ref{}, errors.New("reference must not contain whitespace or control characters")
	}
	switch {
	case strings.HasPrefix(s, "op://"):
		return parseSegments(SchemeOnePassword, strings.TrimPrefix(s, "op://"), 3, 3, "op://vault/item/field")
	case strings.HasPrefix(s, "gcpsm://"):
		return parseSegments(SchemeGCPSecret, strings.TrimPrefix(s, "gcpsm://"), 2, 3, "gcpsm://project/secret[/version]")
	case strings.HasPrefix(s, "keychain:"):
		return parseSegments(SchemeKeychain, strings.TrimPrefix(s, "keychain:"), 1, 1, "keychain:<service>")
	case strings.HasPrefix(s, "env:"):
		name := strings.TrimPrefix(s, "env:")
		if !envVarName.MatchString(name) {
			return Ref{}, errors.New("env reference needs a variable name, e.g. env:OPENAI_API_KEY")
		}
		return Ref{Scheme: SchemeEnv, Path: name}, nil
	}
	return Ref{}, errors.New("unsupported reference; use op://, gcpsm://, keychain: or env:")
}

func parseSegments(scheme, path string, minParts, maxParts int, shape string) (Ref, error) {
	parts := strings.Split(path, "/")
	if scheme == SchemeKeychain {
		parts = []string{path}
	}
	if len(parts) < minParts || len(parts) > maxParts {
		return Ref{}, fmt.Errorf("reference must look like %s", shape)
	}
	for _, part := range parts {
		if part == "" || part == ".." || part == "." {
			return Ref{}, fmt.Errorf("reference must look like %s with no empty or relative parts", shape)
		}
	}
	return Ref{Scheme: scheme, Path: path}, nil
}

// String is the canonical reference text, safe to print and store.
func (r Ref) String() string {
	switch r.Scheme {
	case SchemeOnePassword, SchemeGCPSecret:
		return r.Scheme + "://" + r.Path
	default:
		return r.Scheme + ":" + r.Path
	}
}

// ErrHelperMissing reports that the CLI needed to resolve a reference is not
// on PATH. It is returned as *ErrHelperMissing.
type ErrHelperMissing struct{ Helper string }

func (e ErrHelperMissing) Error() string {
	return fmt.Sprintf("%s not found on PATH; it is needed to resolve the key reference", e.Helper)
}

// Deps are the outside-world hooks Resolve needs.
type Deps struct {
	// Runner returns a runner for the helper executable, or an error wrapping
	// *ErrHelperMissing when it is not installed.
	Runner func(helper string) (provider.Runner, error)
	Getenv func(string) string
	GOOS   string
}

// DefaultDeps looks helpers up on PATH and uses the real environment and OS.
func DefaultDeps() Deps {
	return Deps{
		Runner: func(helper string) (provider.Runner, error) {
			bin, err := exec.LookPath(helper)
			if err != nil {
				return nil, &ErrHelperMissing{Helper: helper}
			}
			return execrunner.New(bin), nil
		},
		Getenv: os.Getenv,
		GOOS:   currentOS(),
	}
}

// Resolve returns the secret the reference points at.
func (r Ref) Resolve(ctx context.Context, deps Deps) (string, error) {
	if r.Scheme == SchemeEnv {
		return r.resolveEnv(deps)
	}
	helper, args, err := r.helperCommand(deps.GOOS)
	if err != nil {
		return "", err
	}
	run, err := deps.Runner(helper)
	if err != nil {
		return "", err
	}
	output, err := run.Query(ctx, nil, args...)
	if err != nil {
		return "", helperFailure(helper, r, err)
	}
	value := strings.TrimSpace(output)
	if value == "" {
		return "", fmt.Errorf("%s returned an empty value for %s", helper, r)
	}
	return value, nil
}

func (r Ref) resolveEnv(deps Deps) (string, error) {
	getenv := deps.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	value := strings.TrimSpace(getenv(r.Path))
	if value == "" {
		return "", fmt.Errorf("environment variable %s is empty or not set", r.Path)
	}
	return value, nil
}

func (r Ref) helperCommand(goos string) (helper string, args []string, err error) {
	switch r.Scheme {
	case SchemeOnePassword:
		return "op", []string{"read", r.String()}, nil
	case SchemeGCPSecret:
		project, secret, version := r.gcpParts()
		return "gcloud", []string{"secrets", "versions", "access", version, "--secret=" + secret, "--project=" + project}, nil
	case SchemeKeychain:
		switch goos {
		case "darwin":
			return "security", []string{"find-generic-password", "-w", "-s", r.Path}, nil
		case "linux":
			return "secret-tool", []string{"lookup", "service", r.Path}, nil
		}
		return "", nil, fmt.Errorf("keychain references are not supported on %s", goos)
	}
	return "", nil, fmt.Errorf("unsupported reference scheme %q", r.Scheme)
}

func (r Ref) gcpParts() (project, secret, version string) {
	parts := strings.Split(r.Path, "/")
	version = latestSecret
	if len(parts) > 2 {
		version = parts[2]
	}
	return parts[0], parts[1], version
}

// helperFailure reports only the helper, the reference and the exit status:
// the wrapped error and its captured output can hold the secret.
func helperFailure(helper string, ref Ref, err error) error {
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s timed out resolving %s", helper, ref)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%s was canceled resolving %s", helper, ref)
	case errors.As(err, &exitErr):
		return fmt.Errorf("%s failed resolving %s: exit status %d", helper, ref, exitErr.ExitCode())
	}
	return fmt.Errorf("%s failed resolving %s", helper, ref)
}

// ShellLookup is the POSIX shell expression that resolves the secret in the
// user's own shell, for the current OS.
func (r Ref) ShellLookup() (string, error) { return r.ShellLookupFor(currentOS()) }

// ShellLookupFor is ShellLookup for an explicit OS.
func (r Ref) ShellLookupFor(goos string) (string, error) {
	if r.Scheme == SchemeEnv {
		return "${" + r.Path + "}", nil
	}
	helper, args, err := r.helperCommand(goos)
	if err != nil {
		return "", err
	}
	words := make([]string, 0, len(args)+1)
	words = append(words, helper)
	for _, arg := range args {
		words = append(words, shellQuote(arg))
	}
	return "$(" + strings.Join(words, " ") + ")", nil
}

// shellQuote single-quotes s unless it is made only of shell-safe characters.
func shellQuote(s string) string {
	if flag, value, isFlag := strings.Cut(s, "="); isFlag && strings.HasPrefix(flag, "--") {
		return flag + "=" + shellQuote(value)
	}
	if s != "" && strings.Trim(s, safeShellChars) == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

const safeShellChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./:@%+=,-"

func currentOS() string { return runtime.GOOS }
