package manager_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/k0wl0n/gctx/internal/testutil"
	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/aws"
	"github.com/k0wl0n/gctx/pkg/provider/azure"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

var (
	fakeAWSOnce sync.Once
	fakeAWSPath string
)

func fakeAWSBinary(t *testing.T) string {
	t.Helper()
	fakeAWSOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gctx-fakeaws-")
		if err != nil {
			t.Fatal(err)
		}
		fakeAWSPath = filepath.Join(dir, "aws")
		if out, err := exec.Command("go", "build", "-o", fakeAWSPath, "github.com/k0wl0n/gctx/internal/testutil/fakeaws").CombinedOutput(); err != nil {
			t.Fatalf("building fakeaws: %v\n%s", err, out)
		}
	})
	return fakeAWSPath
}

// networkManager is a manager over the real aws and azure providers talking
// to the fake CLIs; awsEnv and azEnv are the fakes' behaviour knobs.
func networkManager(t *testing.T, awsEnv, azEnv []string) *manager.Manager {
	t.Helper()
	awsBin, azBin := fakeAWSBinary(t), fakeAZBinary(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "gctx", "azure")
	if err := os.MkdirAll(filepath.Join(root, "corp"), 0o700); err != nil {
		t.Fatal(err)
	}
	paths := manager.Paths{GctxDir: filepath.Join(dir, "gctx"), GcloudDir: filepath.Join(dir, "gcloud")}
	cfg, err := config.LoadFrom(paths.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []config.Account{
		{Name: "prod", Provider: "aws", ProjectID: "prod", Settings: map[string]string{"auth": "profile"}},
		{Name: "corp", Provider: "azure", ProjectID: "12345678-1234-1234-1234-123456789abc", Settings: map[string]string{"dir": filepath.Join(root, "corp")}},
	} {
		if err := cfg.AddAccount(&account); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.NewWithOptions(manager.Options{
		Paths:  paths,
		Getenv: func(string) string { return "" },
		Providers: []provider.Provider{
			aws.New(aws.Options{
				ConfigFile: filepath.Join(dir, "awsconfig"), Getenv: func(string) string { return "" },
				LookPath: func(string) (string, error) { return awsBin, nil },
				Runner:   execrunner.New(awsBin).WithEnv(awsEnv),
			}),
			azure.New(azure.Options{
				ConfigRoot: root, Getenv: func(string) string { return "" },
				LookPath: func(string) (string, error) { return azBin, nil },
				Runner:   execrunner.New(azBin).WithEnv(azEnv),
			}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestOfflineFromFastNetworkErrors(t *testing.T) {
	cases := []struct {
		name          string
		awsEnv, azEnv []string
		want          []provider.ID
	}{
		{"both fail fast", []string{"FAKE_AWS_STS_NETFAIL=1"}, []string{"FAKE_AZ_TOKEN_NETFAIL=1"}, []provider.ID{provider.AWS, provider.Azure}},
		{"one fails fast", []string{"FAKE_AWS_STS_NETFAIL=1"}, nil, []provider.ID{provider.AWS}},
		{"all reachable", nil, nil, nil},
		{"expired credentials are an answer, not an outage", []string{"FAKE_AWS_STS_FAIL=1"}, []string{"FAKE_AZ_TOKEN_FAIL=1"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := networkManager(t, c.awsEnv, c.azEnv)
			report, err := m.CheckCredentials(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			got := slices.Clone(report.NetworkFailed)
			slices.Sort(got)
			if !slices.Equal(got, c.want) {
				t.Errorf("NetworkFailed %v, want %v", got, c.want)
			}
			if len(report.Warnings) != 0 {
				t.Errorf("a fast network error needs no warning: %v", report.Warnings)
			}
			for _, id := range []provider.ID{provider.AWS, provider.Azure} {
				one, err := m.CheckProviderCredentials(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				if slices.Contains(one.NetworkFailed, id) != slices.Contains(c.want, id) {
					t.Errorf("per-provider check of %s: NetworkFailed %v", id, one.NetworkFailed)
				}
			}
		})
	}
}

func TestNetworkAuthMetadata(t *testing.T) {
	m := networkManager(t, nil, nil)
	for _, info := range m.Providers() {
		if !info.NetworkAuth {
			t.Errorf("%s checks over the network", info.ID)
		}
	}
	gcp, _ := defaultSetManager(t, nil)
	for _, info := range gcp.Providers() {
		want := info.ID != provider.GCP
		if info.NetworkAuth != want {
			t.Errorf("%s NetworkAuth %v, want %v", info.ID, info.NetworkAuth, want)
		}
	}
}

func TestGCPCheckNeverReportsNetworkFailure(t *testing.T) {
	env := testutil.NewEnv(t)
	env.SeedAccount("work", "p1", testutil.SeedOptions{})
	t.Setenv("FAKE_GCLOUD_FAIL", "auth-list:1:ERROR: boom")
	report, err := newManager(t, env).CheckCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.NetworkFailed) != 0 {
		t.Errorf("gcloud auth list is local; NetworkFailed %v", report.NetworkFailed)
	}
}
