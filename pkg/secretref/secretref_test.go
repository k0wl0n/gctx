package secretref_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
	"github.com/k0wl0n/gctx/pkg/secretref"
)

const fakeKey = "FAKEKEY-do-not-use-1234567890"

type recordingRunner struct {
	calls  *[][]string
	output string
	err    error
}

func (r recordingRunner) Query(_ context.Context, _ []string, args ...string) (string, error) {
	*r.calls = append(*r.calls, args)
	return r.output, r.err
}

func (r recordingRunner) Interactive(context.Context, provider.Stdio, []string, ...string) error {
	return errors.New("not used")
}

func (r recordingRunner) WithEnv([]string) provider.Runner { return r }

// recordingDeps returns deps whose helper runners record "<helper> <args>".
func recordingDeps(goos, output string, calls *[]string) secretref.Deps {
	return secretref.Deps{
		Runner: func(helper string) (provider.Runner, error) {
			return helperRunner{helper: helper, calls: calls, output: output}, nil
		},
		Getenv: func(string) string { return "" },
		GOOS:   goos,
	}
}

type helperRunner struct {
	helper string
	calls  *[]string
	output string
}

func (r helperRunner) Query(_ context.Context, _ []string, args ...string) (string, error) {
	*r.calls = append(*r.calls, r.helper+" "+strings.Join(args, " "))
	return r.output, nil
}

func (r helperRunner) Interactive(context.Context, provider.Stdio, []string, ...string) error {
	return errors.New("not used")
}

func (r helperRunner) WithEnv([]string) provider.Runner { return r }

func fakeOpDeps(t *testing.T) secretref.Deps {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "op")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/k0wl0n/gctx/internal/testutil/fakeop").CombinedOutput(); err != nil {
		t.Fatalf("building fakeop: %v\n%s", err, out)
	}
	return secretref.Deps{
		Runner: func(helper string) (provider.Runner, error) {
			if helper != "op" {
				return nil, secretref.ErrHelperMissing{Helper: helper}
			}
			return execrunner.New(bin), nil
		},
		Getenv: os.Getenv,
		GOOS:   runtime.GOOS,
	}
}

func mustParse(t *testing.T, s string) secretref.Ref {
	t.Helper()
	ref, err := secretref.Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return ref
}

func TestParseSchemes(t *testing.T) {
	valid := []struct{ in, scheme, path string }{
		{"op://Private/sumopod-dev/credential", "op", "Private/sumopod-dev/credential"},
		{"gcpsm://aliz-ai/sumopod-prod", "gcpsm", "aliz-ai/sumopod-prod"},
		{"gcpsm://aliz-ai/sumopod-prod/3", "gcpsm", "aliz-ai/sumopod-prod/3"},
		{"keychain:openrouter", "keychain", "openrouter"},
		{"env:OPENAI_API_KEY", "env", "OPENAI_API_KEY"},
		{"env:_x1", "env", "_x1"},
	}
	for _, c := range valid {
		ref, err := secretref.Parse(c.in)
		if err != nil || ref.Scheme != c.scheme || ref.Path != c.path {
			t.Errorf("Parse(%q) = %+v, %v", c.in, ref, err)
		}
	}

	invalid := []string{
		"", "op://", "op://myvault/myitem", "op://vault/item/field/extra", "op://vault//field",
		"op://vault/../field", "op://vault/item/ field", "op://vault/item/fi\teld", "op://vault/item/fi\x01eld",
		"gcpsm://proj", "gcpsm://proj/secret/1/2", "gcpsm:///secret", "gcpsm://proj/..",
		"keychain:", "keychain:has space", "keychain:..",
		"env:", "env:1BAD", "env:HAS-DASH", "env:HAS SPACE",
		"vault://x/y/z", "plain-text", "https://example.com/a",
	}
	for _, in := range invalid {
		_, err := secretref.Parse(in)
		if err == nil {
			t.Errorf("Parse(%q) accepted", in)
			continue
		}
		if len(in) > len("gcpsm://proj") && strings.Contains(err.Error(), in) {
			t.Errorf("Parse(%q) error echoes the input: %v", in, err)
		}
	}
}

func TestParseRejectsRawKeys(t *testing.T) {
	for _, raw := range []string{
		"sk-proj-abcdefghijklmnop",
		"sk-or-v1-abcdef0123456789",
		"abcdefghijklmnopqrstuvwxyzABCDEF01234567",
	} {
		_, err := secretref.Parse(raw)
		if err == nil || err.Error() != "store a reference, not the key" {
			t.Errorf("Parse(raw key) error = %v", err)
		}
	}
}

func TestStringIsCanonicalAndSafe(t *testing.T) {
	for _, in := range []string{
		"op://Private/sumopod-dev/credential",
		"gcpsm://aliz-ai/sumopod-prod/3",
		"keychain:openrouter",
		"env:OPENAI_API_KEY",
	} {
		if got := mustParse(t, in).String(); got != in {
			t.Errorf("String() = %q, want %q", got, in)
		}
	}
}

func TestResolveOp(t *testing.T) {
	deps := fakeOpDeps(t)
	logPath := filepath.Join(t.TempDir(), "op.log")
	t.Setenv("FAKE_OP_LOG", logPath)
	got, err := mustParse(t, "op://Private/sumopod-dev/credential").Resolve(context.Background(), deps)
	if err != nil || got != fakeKey {
		t.Fatalf("got err %v (value match: %v)", err, got == fakeKey)
	}
	logged, _ := os.ReadFile(logPath)
	if !strings.Contains(string(logged), `["read","op://Private/sumopod-dev/credential"]`) {
		t.Errorf("op invocation log %s", logged)
	}
}

func TestResolveGCPSecretManager(t *testing.T) {
	var calls []string
	deps := recordingDeps("linux", "value", &calls)
	ctx := context.Background()
	if _, err := mustParse(t, "gcpsm://aliz-ai/sumopod-prod/7").Resolve(ctx, deps); err != nil {
		t.Fatal(err)
	}
	if _, err := mustParse(t, "gcpsm://aliz-ai/sumopod-prod").Resolve(ctx, deps); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"gcloud secrets versions access 7 --secret=sumopod-prod --project=aliz-ai",
		"gcloud secrets versions access latest --secret=sumopod-prod --project=aliz-ai",
	}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Errorf("calls %q, want %q", calls, want)
	}
}

func TestResolveKeychainDarwinAndLinux(t *testing.T) {
	ref := mustParse(t, "keychain:openrouter")
	cases := []struct{ goos, want string }{
		{"darwin", "security find-generic-password -w -s openrouter"},
		{"linux", "secret-tool lookup service openrouter"},
	}
	for _, c := range cases {
		var calls []string
		if _, err := ref.Resolve(context.Background(), recordingDeps(c.goos, "value", &calls)); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 1 || calls[0] != c.want {
			t.Errorf("%s: calls %q, want %q", c.goos, calls, c.want)
		}
	}

	var calls []string
	_, err := ref.Resolve(context.Background(), recordingDeps("plan9", "value", &calls))
	if err == nil || !strings.Contains(err.Error(), "plan9") || len(calls) != 0 {
		t.Errorf("unsupported OS: err %v calls %q", err, calls)
	}
}

func TestResolveEnvMissing(t *testing.T) {
	ref := mustParse(t, "env:OPENAI_API_KEY")
	deps := secretref.Deps{Getenv: func(string) string { return "" }}
	_, err := ref.Resolve(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("err %v does not name the variable", err)
	}

	deps.Getenv = func(name string) string {
		if name == "OPENAI_API_KEY" {
			return fakeKey
		}
		return ""
	}
	got, err := ref.Resolve(context.Background(), deps)
	if err != nil || got != fakeKey {
		t.Fatalf("set variable: err %v", err)
	}
}

func TestResolveHelperMissingIsTyped(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := mustParse(t, "op://Private/sumopod-dev/credential").Resolve(context.Background(), secretref.DefaultDeps())
	var missing secretref.ErrHelperMissing
	if !errors.As(err, &missing) || missing.Helper != "op" {
		t.Fatalf("err %v is not ErrHelperMissing{op}", err)
	}
}

func TestHelperMissingMatchesValueTarget(t *testing.T) {
	var value secretref.ErrHelperMissing
	if !errors.As(secretref.ErrHelperMissing{Helper: "op"}, &value) || value.Helper != "op" {
		t.Error("value target does not match")
	}
	// The documented form is the value type; a pointer target is not matched.
	var pointer *secretref.ErrHelperMissing
	if errors.As(secretref.ErrHelperMissing{Helper: "op"}, &pointer) {
		t.Error("pointer target unexpectedly matches")
	}
}

func TestResolveWithNilRunnerFallsBackToPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := mustParse(t, "keychain:svc").Resolve(context.Background(), secretref.Deps{GOOS: "linux"})
	var missing secretref.ErrHelperMissing
	if !errors.As(err, &missing) {
		t.Fatalf("err %v, want ErrHelperMissing", err)
	}
}

func TestParseRejectsFlagLikeSegments(t *testing.T) {
	for _, in := range []string{
		"gcpsm://p/s/--impersonate-service-account=x",
		"gcpsm://p/s/-1",
		"gcpsm://p/s/v1",
		"gcpsm://-p/s",
		"gcpsm://p/-s",
		"keychain:-x",
		"op://-x/item/field",
	} {
		if _, err := secretref.Parse(in); err == nil {
			t.Errorf("Parse(%q) accepted", in)
		}
	}
}

func TestParseRejectsShellMetacharacters(t *testing.T) {
	for _, in := range []string{
		"op://v/i/f;touch", "op://v/i/$(id)", "op://v/i/`id`", "op://v/i/f\nx", "op://v/i/f\x00x", "op://v/i/f x",
		"gcpsm://p;x/s", "gcpsm://p/s$(id)", "gcpsm://p/s`id`/3", "gcpsm://p/s x",
		"keychain:a;b", "keychain:a$b", "keychain:a`b`", "keychain:a\nb", "keychain:a b", "keychain:a'b",
	} {
		if _, err := secretref.Parse(in); err == nil {
			t.Errorf("Parse(%q) accepted", in)
		}
	}
}

func TestShellLookupNeverLeavesUnquotedUserInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	hostile := []string{"a'b", "a b", "a$b", "a;touch x", "a`id`", "a\"b", "a--x;touch=1"}
	for _, value := range hostile {
		refs := []struct {
			ref  secretref.Ref
			goos string
			want []string
		}{
			{secretref.Ref{Scheme: "op", Path: "v/i/" + value}, "linux", []string{"op", "read", "op://v/i/" + value}},
			{secretref.Ref{Scheme: "gcpsm", Path: "p/" + value + "/3"}, "linux", []string{"gcloud", "secrets", "versions", "access", "3", "--secret=" + value, "--project=p"}},
			{secretref.Ref{Scheme: "gcpsm", Path: "p/s/" + value}, "linux", []string{"gcloud", "secrets", "versions", "access", value, "--secret=s", "--project=p"}},
			{secretref.Ref{Scheme: "keychain", Path: value}, "darwin", []string{"security", "find-generic-password", "-w", "-s", value}},
		}
		for _, c := range refs {
			expr, err := c.ref.ShellLookupFor(c.goos)
			if err != nil {
				t.Errorf("%+v: %v", c.ref, err)
				continue
			}
			inner := strings.TrimSuffix(strings.TrimPrefix(expr, "$("), ")")
			script := `printf '%s\n' ` + strings.TrimPrefix(inner, strings.Fields(inner)[0])
			out, err := exec.Command("/bin/sh", "-c", script).Output()
			if err != nil {
				t.Errorf("%q does not parse: %v", expr, err)
				continue
			}
			got := append([]string{c.want[0]}, strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")...)
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("%q round-trips to %q, want %q", expr, got, c.want)
			}
		}
	}
}

func TestShellLookupRefusesLeadingDash(t *testing.T) {
	for _, ref := range []secretref.Ref{
		{Scheme: "keychain", Path: "-x"},
		{Scheme: "gcpsm", Path: "p/s/--x=1"},
		{Scheme: "gcpsm", Path: "-p/s"},
		{Scheme: "op", Path: "-v/i/f"},
	} {
		if _, err := ref.ShellLookupFor("darwin"); err == nil {
			t.Errorf("%+v accepted", ref)
		}
	}
}

func TestResolveTrimsNewlineAndRejectsEmpty(t *testing.T) {
	ref := mustParse(t, "op://Private/sumopod-dev/credential")
	var calls []string
	got, err := ref.Resolve(context.Background(), recordingDeps("linux", "  value\r\n\n", &calls))
	if err != nil || got != "value" {
		t.Errorf("trim: err %v, value match %v", err, got == "value")
	}
	for _, empty := range []string{"", "\n", " \r\n"} {
		if _, err := ref.Resolve(context.Background(), recordingDeps("linux", empty, &calls)); err == nil {
			t.Errorf("empty output %q accepted", empty)
		}
	}
}

func TestShellLookupMockupForm(t *testing.T) {
	got, err := mustParse(t, "op://Private/sumopod-dev/credential").ShellLookup()
	want := "$(op read op://Private/sumopod-dev/credential)"
	if err != nil || got != want {
		t.Errorf("got %q, %v want %q", got, err, want)
	}
}

func TestShellLookupForms(t *testing.T) {
	cases := []struct{ ref, goos, want string }{
		{"gcpsm://aliz-ai/sumopod-prod", "linux", "$(gcloud secrets versions access latest --secret=sumopod-prod --project=aliz-ai)"},
		{"gcpsm://aliz-ai/sumopod-prod/3", "linux", "$(gcloud secrets versions access 3 --secret=sumopod-prod --project=aliz-ai)"},
		{"keychain:openrouter", "darwin", "$(security find-generic-password -w -s openrouter)"},
		{"keychain:openrouter", "linux", "$(secret-tool lookup service openrouter)"},
		{"env:OPENAI_API_KEY", "linux", "${OPENAI_API_KEY}"},
	}
	for _, c := range cases {
		got, err := mustParse(t, c.ref).ShellLookupFor(c.goos)
		if err != nil || got != c.want {
			t.Errorf("%s/%s: got %q, %v want %q", c.ref, c.goos, got, err, c.want)
		}
	}
	if _, err := mustParse(t, "keychain:x").ShellLookupFor("plan9"); err == nil {
		t.Error("unsupported OS accepted")
	}
}

func TestShellLookupQuoting(t *testing.T) {
	// Parse forbids whitespace, so build refs directly for hostile paths.
	cases := []struct {
		ref  secretref.Ref
		want string
	}{
		{secretref.Ref{Scheme: "op", Path: "Priv ate/it'em/$field"}, `$(op read 'op://Priv ate/it'\''em/$field')`},
		{secretref.Ref{Scheme: "keychain", Path: "my svc"}, `$(security find-generic-password -w -s 'my svc')`},
		{secretref.Ref{Scheme: "gcpsm", Path: "p/s$x"}, `$(gcloud secrets versions access latest --secret='s$x' --project=p)`},
	}
	for _, c := range cases {
		got, err := c.ref.ShellLookupFor("darwin")
		if err != nil || got != c.want {
			t.Errorf("got %q, %v want %q", got, err, c.want)
		}
	}
}

func TestErrorsNeverContainValue(t *testing.T) {
	t.Setenv("FAKE_OP_FAIL", "1")
	deps := fakeOpDeps(t)
	_, err := mustParse(t, "op://Private/sumopod-dev/credential").Resolve(context.Background(), deps)
	if err == nil {
		t.Fatal("expected failure")
	}
	assertNoValue(t, err)
	if !strings.Contains(err.Error(), "op") || !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("error lacks helper name or status: %v", err)
	}

	leaky := &provider.CommandError{Step: "read op://x/y/z", Err: errors.New("exit status 1"), Output: fakeKey}
	var calls [][]string
	deps = secretref.Deps{
		Runner: func(string) (provider.Runner, error) {
			return recordingRunner{calls: &calls, output: fakeKey, err: leaky}, nil
		},
		GOOS: "linux",
	}
	_, err = mustParse(t, "op://Private/sumopod-dev/credential").Resolve(context.Background(), deps)
	if err == nil {
		t.Fatal("expected failure")
	}
	assertNoValue(t, err)
	var cmdErr *provider.CommandError
	if errors.As(err, &cmdErr) {
		t.Errorf("error still wraps the CommandError holding the output")
	}
}

func assertNoValue(t *testing.T, err error) {
	t.Helper()
	renderings := []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)}
	for inner := errors.Unwrap(err); inner != nil; inner = errors.Unwrap(inner) {
		renderings = append(renderings, fmt.Sprintf("%+v", inner), fmt.Sprintf("%#v", inner))
	}
	for _, text := range renderings {
		if strings.Contains(text, fakeKey) {
			t.Fatal("an error rendering contains the secret value")
		}
	}
}
