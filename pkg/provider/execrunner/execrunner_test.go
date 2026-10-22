package execrunner_test

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

func shell(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	return "/bin/sh"
}

func TestQueryReturnsTrimmedStdout(t *testing.T) {
	runner := execrunner.New(shell(t))
	out, err := runner.Query(context.Background(), nil, "-c", "echo hello; echo noise >&2")
	if err != nil || out != "hello" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestQueryFailureCarriesCommandError(t *testing.T) {
	runner := execrunner.New(shell(t))
	_, err := runner.Query(context.Background(), nil, "-c", "echo boom >&2; exit 3")
	var cmdErr *provider.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err %v is not a CommandError", err)
	}
	if cmdErr.Step != "-c echo boom >&2; exit 3" || cmdErr.LastLine() != "boom" {
		t.Errorf("step %q last line %q", cmdErr.Step, cmdErr.LastLine())
	}
}

func TestQueryHonoursContextDeadline(t *testing.T) {
	runner := execrunner.New(shell(t))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runner.Query(ctx, nil, "-c", "sleep 30")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("took %s", elapsed)
	}
}

func TestWithEnvAppliesToEveryChild(t *testing.T) {
	runner := execrunner.New(shell(t)).WithEnv([]string{"EXECRUNNER_A=one"})
	out, err := runner.Query(context.Background(), []string{"EXECRUNNER_B=two"}, "-c", `echo "$EXECRUNNER_A $EXECRUNNER_B"`)
	if err != nil || out != "one two" {
		t.Fatalf("out %q err %v", out, err)
	}

	var stdout bytes.Buffer
	err = runner.Interactive(context.Background(), provider.Stdio{Stdout: &stdout}, nil, "-c", `echo "$EXECRUNNER_A"`)
	if err != nil || strings.TrimSpace(stdout.String()) != "one" {
		t.Fatalf("interactive out %q err %v", stdout.String(), err)
	}
}

func TestInteractivePassesStdio(t *testing.T) {
	runner := execrunner.New(shell(t))
	var stdout bytes.Buffer
	err := runner.Interactive(context.Background(),
		provider.Stdio{Stdin: strings.NewReader("typed\n"), Stdout: &stdout}, nil, "-c", "read x; echo got-$x")
	if err != nil || strings.TrimSpace(stdout.String()) != "got-typed" {
		t.Fatalf("out %q err %v", stdout.String(), err)
	}
}
