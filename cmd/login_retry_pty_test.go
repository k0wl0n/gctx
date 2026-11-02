//go:build !windows

package cmd

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/internal/testutil"
)

// ptyScript drives a command on a PTY: expect waits for output, send types.
type ptyScript struct {
	t    *testing.T
	pty  *os.File
	mu   sync.Mutex
	seen strings.Builder
}

func newPTYScript(t *testing.T, cmd *exec.Cmd) *ptyScript {
	t.Helper()
	s := &ptyScript{t: t, pty: ptyRun(t, cmd)}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.pty.Read(buf)
			s.mu.Lock()
			s.seen.Write(buf[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *ptyScript) output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen.String()
}

func (s *ptyScript) expect(text string) {
	s.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.output(), text) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("timed out waiting for %q in:\n%s", text, s.output())
}

func (s *ptyScript) send(text string) { io.WriteString(s.pty, text) }

// startLoginOnPTY starts `gctx login work` on a PTY. The binary is built by
// the caller first: building under the test HOME would fill the temp dir with
// a read-only module cache.
func startLoginOnPTY(t *testing.T, bin string, env *testutil.Env, failures string) (*exec.Cmd, *ptyScript) {
	t.Helper()
	env.SeedAccount("work", "p1", testutil.SeedOptions{NoADC: true})
	t.Setenv("FAKE_GCLOUD_ADC_MISSING_CODE", failures)
	cmd := exec.Command(bin, "login", "work")
	cmd.Env = os.Environ()
	return cmd, newPTYScript(t, cmd)
}

func gcloudVerbs(env *testutil.Env) []string {
	var verbs []string
	for _, call := range env.Invocations() {
		verbs = append(verbs, call.Verb)
	}
	return verbs
}

func countVerb(verbs []string, verb string) int {
	count := 0
	for _, v := range verbs {
		if v == verb {
			count++
		}
	}
	return count
}

func TestLoginADCRetryOnlyRerunsADCStep(t *testing.T) {
	bin := gctxBinary(t)
	env := testutil.NewEnv(t)
	cmd, script := startLoginOnPTY(t, bin, env, "1")

	script.expect("Retry ADC login only? [Y/n]")
	script.send("y\n")
	if err := cmd.Wait(); err != nil {
		t.Fatalf("login after retry: %v\n%s", err, script.output())
	}
	out := script.output()
	if !strings.Contains(out, "Close any old Google sign-in tabs") {
		t.Errorf("explanation missing: %s", out)
	}
	verbs := gcloudVerbs(env)
	if got := countVerb(verbs, "auth-login"); got != 1 {
		t.Errorf("auth login ran %d times, want 1: %v", got, verbs)
	}
	if got := countVerb(verbs, "auth-adc-login"); got != 2 {
		t.Errorf("adc login ran %d times, want 2: %v", got, verbs)
	}
	if env.ReadConfig().Accounts["work"].ADCPath != env.StoredADCPath("work") {
		t.Error("adc_path not recorded after retry")
	}
}

func TestLoginADCRetryDeclinedExitsWithError(t *testing.T) {
	bin := gctxBinary(t)
	env := testutil.NewEnv(t)
	cmd, script := startLoginOnPTY(t, bin, env, "1")

	script.expect("Retry ADC login only? [Y/n]")
	script.send("n\n")
	err := cmd.Wait()
	if err == nil {
		t.Fatalf("expected failure exit:\n%s", script.output())
	}
	script.expect("ADC auth failed: exit status 1")
	if got := countVerb(gcloudVerbs(env), "auth-adc-login"); got != 1 {
		t.Errorf("adc login ran %d times, want 1", got)
	}
}
