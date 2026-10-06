package gcloud_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/internal/testutil"
	"github.com/k0wl0n/gctx/pkg/gcloud"
)

func TestOutputIsNonInteractive(t *testing.T) {
	env := testutil.NewEnv(t)
	client := &gcloud.Client{Bin: env.GcloudBin()}

	out, err := client.Output(context.Background(), time.Minute, []string{"CLOUDSDK_ACTIVE_CONFIG_NAME=work-config"}, "config", "get-value", "project")
	if err != nil {
		t.Fatal(err, out)
	}
	calls := env.Invocations()
	if len(calls) != 1 {
		t.Fatalf("calls %+v", calls)
	}
	call := calls[0]
	if !call.StdinDevNull || call.DisablePrompts != "1" || call.ActiveConfig != "work-config" {
		t.Errorf("invocation %+v", call)
	}
}

func TestOutputExplicitConfigOverridesParent(t *testing.T) {
	env := testutil.NewEnv(t)
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "personal-config")
	client := &gcloud.Client{Bin: env.GcloudBin()}
	client.Output(context.Background(), time.Minute, []string{"CLOUDSDK_ACTIVE_CONFIG_NAME=work-config"}, "config", "get-value", "project")
	if got := env.Invocations()[0].ActiveConfig; got != "work-config" {
		t.Errorf("active config %q", got)
	}
}

func TestOutputStepError(t *testing.T) {
	env := testutil.NewEnv(t)
	t.Setenv("FAKE_GCLOUD_FAIL", "config-set:2:ERROR: Reauthentication required.")
	client := &gcloud.Client{Bin: env.GcloudBin()}
	_, err := client.Output(context.Background(), time.Minute, nil, "config", "set", "project", "p")
	var stepErr *gcloud.StepError
	if !errors.As(err, &stepErr) {
		t.Fatalf("err %v", err)
	}
	if err.Error() != "exit status 2" || stepErr.Step != "config set project p" || stepErr.LastLine() != "ERROR: Reauthentication required." {
		t.Errorf("%q %q %q", err, stepErr.Step, stepErr.LastLine())
	}
}

func TestOutputTimeout(t *testing.T) {
	env := testutil.NewEnv(t)
	t.Setenv("FAKE_GCLOUD_SLEEP", "configurations-activate:5000")
	client := &gcloud.Client{Bin: env.GcloudBin()}
	start := time.Now()
	_, err := client.Output(context.Background(), 200*time.Millisecond, nil, "config", "configurations", "activate", "x")
	if err == nil || !strings.Contains(err.Error(), "gcloud config configurations activate x timed out") {
		t.Fatalf("err %v", err)
	}
	if time.Since(start) > 4*time.Second {
		t.Error("process not killed on timeout")
	}
}

func TestInteractive(t *testing.T) {
	env := testutil.NewEnv(t)
	client := &gcloud.Client{Bin: env.GcloudBin()}
	var stdout bytes.Buffer
	err := client.Interactive(context.Background(), gcloud.Stdio{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stdout}, nil, "storage", "ls")
	if err != nil || !strings.Contains(stdout.String(), "fake-gcloud: storage ls") {
		t.Fatalf("%v %q", err, stdout.String())
	}
}

func TestNotFound(t *testing.T) {
	client := &gcloud.Client{}
	if client.Available() {
		t.Fatal("empty Bin must be unavailable")
	}
	if _, err := client.Output(context.Background(), time.Second, nil, "x"); !errors.Is(err, gcloud.ErrNotFound) {
		t.Errorf("err %v", err)
	}
}
