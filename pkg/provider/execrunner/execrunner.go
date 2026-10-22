// Package execrunner implements provider.Runner for CLIs run as child processes.
package execrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/k0wl0n/gctx/pkg/provider"
)

// killGrace bounds how long a cancelled child may keep its output pipes open.
const killGrace = 2 * time.Second

// ErrNoBinary reports a Runner created without a binary.
var ErrNoBinary = errors.New("execrunner: no binary to run")

type runner struct {
	bin   string
	extra []string
}

// New returns a Runner for the CLI at bin. Timeouts come from the context
// deadline of each call; failures are *provider.CommandError.
func New(bin string) provider.Runner { return runner{bin: bin} }

func (r runner) environment(env []string) []string {
	merged := append(os.Environ(), r.extra...)
	return append(merged, env...)
}

func (r runner) WithEnv(extra []string) provider.Runner {
	combined := append(append([]string(nil), r.extra...), extra...)
	return runner{bin: r.bin, extra: combined}
}

func (r runner) Query(ctx context.Context, env []string, args ...string) (string, error) {
	if r.bin == "" {
		return "", ErrNoBinary
	}
	var stdout, stderr bytes.Buffer
	cmd := r.command(ctx, env, args)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", r.failure(ctx, args, err, stdout.String()+stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (r runner) Interactive(ctx context.Context, stdio provider.Stdio, env []string, args ...string) error {
	if r.bin == "" {
		return ErrNoBinary
	}
	cmd := r.command(ctx, env, args)
	cmd.Stdin = stdio.Stdin
	cmd.Stdout = stdio.Stdout
	cmd.Stderr = stdio.Stderr
	if err := cmd.Run(); err != nil {
		return r.failure(ctx, args, err, "")
	}
	return nil
}

func (r runner) command(ctx context.Context, env, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = r.environment(env)
	cmd.WaitDelay = killGrace
	return cmd
}

func (r runner) failure(ctx context.Context, args []string, err error, output string) error {
	step := strings.Join(args, " ")
	if ctxErr := ctx.Err(); ctxErr != nil {
		err = fmt.Errorf("%s %s: %w", filepath.Base(r.bin), step, ctxErr)
	}
	return &provider.CommandError{Step: step, Err: err, Output: output}
}
