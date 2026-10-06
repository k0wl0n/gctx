package gcloud

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNotFound is returned when no gcloud executable is available.
var ErrNotFound = errors.New("gcloud not found on PATH")

// Default timeouts for non-interactive invocations.
const (
	DefaultTimeout    = 30 * time.Second
	SetProjectTimeout = 60 * time.Second
)

// Client runs the gcloud CLI.
type Client struct {
	// Bin is the gcloud executable; empty means gcloud is not installed.
	Bin string
}

// NewClient locates gcloud on PATH.
func NewClient() *Client {
	bin, err := exec.LookPath("gcloud")
	if err != nil {
		return &Client{}
	}
	return &Client{Bin: bin}
}

// Available reports whether a gcloud executable was found.
func (c *Client) Available() bool {
	return c != nil && c.Bin != ""
}

// StepError describes a failed gcloud invocation. Its message is the
// underlying error's, so existing CLI error text is unchanged.
type StepError struct {
	Step   string // gcloud arguments, e.g. "auth login"
	Err    error
	Output string // combined output of non-interactive invocations
}

func (e *StepError) Error() string { return e.Err.Error() }
func (e *StepError) Unwrap() error { return e.Err }

// LastLine returns the last non-empty output line, for compact UI messages.
func (e *StepError) LastLine() string {
	lines := strings.Split(strings.TrimSpace(e.Output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Stdio is the terminal handed to interactive invocations.
type Stdio struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Output runs a non-interactive gcloud command with stdin on the null device,
// prompts disabled, and a timeout. It returns the combined output.
func (c *Client) Output(ctx context.Context, timeout time.Duration, env []string, args ...string) (string, error) {
	if !c.Available() {
		return "", ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Env = append(os.Environ(), append([]string{"CLOUDSDK_CORE_DISABLE_PROMPTS=1"}, env...)...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("gcloud %s timed out after %s", strings.Join(args, " "), timeout)
	}
	if err != nil {
		return output.String(), &StepError{Step: strings.Join(args, " "), Err: err, Output: output.String()}
	}
	return output.String(), nil
}

// Interactive runs gcloud attached to the given terminal, without a timeout.
func (c *Client) Interactive(ctx context.Context, stdio Stdio, env []string, args ...string) error {
	if !c.Available() {
		return ErrNotFound
	}
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Stdin = stdio.Stdin
	cmd.Stdout = stdio.Stdout
	cmd.Stderr = stdio.Stderr
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Run(); err != nil {
		return &StepError{Step: strings.Join(args, " "), Err: err}
	}
	return nil
}

// Command returns an unstarted gcloud command with extra environment.
func (c *Client) Command(env []string, args ...string) *exec.Cmd {
	cmd := exec.Command(c.Bin, args...)
	cmd.Env = append(os.Environ(), env...)
	return cmd
}
