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
	"sync"
	"time"

	"github.com/k0wl0n/gctx/pkg/provider"
)

// killGrace bounds how long a cancelled child may keep its output pipes open.
const killGrace = 2 * time.Second

// streamTailLines is how many trailing lines a failed Stream keeps as the error's output.
const streamTailLines = 20

// ErrNoBinary reports a Runner created without a binary.
var ErrNoBinary = errors.New("execrunner: no binary to run")

type runner struct {
	bin   string
	extra []string
}

// New returns a Runner for the CLI at bin. Timeouts come from the context
// deadline of each call; failures are *provider.CommandError.
func New(bin string) provider.Runner { return runner{bin: bin} }

var _ provider.StreamingRunner = runner{}

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

// Stream runs the command with piped output and calls onLine for every line,
// serialized, as it arrives. A final line without a newline is delivered when
// the child exits. Cancelling ctx kills the child.
func (r runner) Stream(ctx context.Context, env []string, onLine func(stream, line string), args ...string) error {
	if r.bin == "" {
		return ErrNoBinary
	}
	var mu sync.Mutex
	var tail []string
	deliver := func(stream string) func(string) {
		return func(line string) {
			mu.Lock()
			defer mu.Unlock()
			tail = append(tail, line)
			if len(tail) > streamTailLines {
				tail = tail[1:]
			}
			onLine(stream, line)
		}
	}
	stdout, stderr := &lineWriter{emit: deliver(provider.StreamStdout)}, &lineWriter{emit: deliver(provider.StreamStderr)}
	cmd := r.command(ctx, env, args)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	stdout.flush()
	stderr.flush()
	if err != nil {
		mu.Lock()
		output := strings.Join(tail, "\n")
		mu.Unlock()
		return r.failure(ctx, args, err, output)
	}
	return nil
}

// lineWriter splits written bytes into lines. It is used by one os/exec copy
// goroutine, so it needs no locking of its own.
type lineWriter struct {
	emit func(string)
	buf  []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		end := bytes.IndexByte(w.buf, '\n')
		if end < 0 {
			return len(p), nil
		}
		w.emit(strings.TrimRight(string(w.buf[:end]), "\r"))
		w.buf = w.buf[end+1:]
	}
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(strings.TrimRight(string(w.buf), "\r"))
		w.buf = nil
	}
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
