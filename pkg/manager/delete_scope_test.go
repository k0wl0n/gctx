package manager_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/azure"
	"github.com/k0wl0n/gctx/pkg/provider/execrunner"
)

var (
	fakeAZOnce sync.Once
	fakeAZPath string
)

func fakeAZBinary(t *testing.T) string {
	t.Helper()
	fakeAZOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gctx-fakeaz-")
		if err != nil {
			t.Fatal(err)
		}
		fakeAZPath = filepath.Join(dir, "az")
		if out, err := exec.Command("go", "build", "-o", fakeAZPath, "github.com/k0wl0n/gctx/internal/testutil/fakeaz").CombinedOutput(); err != nil {
			t.Fatalf("building fakeaz: %v\n%s", err, out)
		}
	})
	return fakeAZPath
}

type azCall struct {
	Args            []string `json:"args"`
	ConfigDirExists bool     `json:"config_dir_exists"`
}

func TestDeleteLogoutRunsBeforeRemoval(t *testing.T) {
	bin := fakeAZBinary(t)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	root := filepath.Join(dir, "gctx", "azure")
	accountDir := filepath.Join(root, "corp")
	if err := os.MkdirAll(accountDir, 0o700); err != nil {
		t.Fatal(err)
	}
	az := azure.New(azure.Options{
		ConfigRoot: root,
		LookPath:   func(string) (string, error) { return bin, nil },
		Getenv:     func(string) string { return "" },
		Runner:     execrunner.New(bin).WithEnv([]string{"FAKE_AZ_LOG=" + logPath}),
	})
	account := config.Account{Name: "corp", ProjectID: "12345678-1234-1234-1234-123456789abc", Provider: "azure", Settings: map[string]string{"dir": accountDir}}
	m, _ := dispatchManager(t, []config.Account{account}, az)

	if _, err := m.Delete(context.Background(), "corp", manager.DeleteOptions{Logout: true, RemoveDir: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(accountDir); !os.IsNotExist(err) {
		t.Fatalf("dir should be removed: %v", err)
	}
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var logout *azCall
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var call azCall
		if err := json.Unmarshal(scanner.Bytes(), &call); err != nil {
			t.Fatal(err)
		}
		if len(call.Args) > 0 && call.Args[0] == "logout" {
			logout = &call
		}
	}
	if logout == nil {
		t.Fatal("az logout was not run")
	}
	if !logout.ConfigDirExists {
		t.Error("logout ran after the isolated dir was removed")
	}
}

func TestDeleteRefusedWhenLinked(t *testing.T) {
	cloud, tool := newFake(provider.GCP, "PROJECT"), newFake(provider.AWS, "PROFILE")
	accounts := []config.Account{
		{Name: "cloud", ConfigName: "cloud-config", ProjectID: "proj"},
		{Name: "zeta", Provider: "aws", ProjectID: "p", Settings: map[string]string{"via": "cloud"}},
		{Name: "alpha", Provider: "aws", ProjectID: "p", Settings: map[string]string{"via": "cloud"}},
		{Name: "other", Provider: "aws", ProjectID: "p"},
	}
	m, _ := dispatchManager(t, accounts, cloud, tool)

	_, err := m.Delete(context.Background(), "cloud", manager.DeleteOptions{})
	var linked manager.ErrAccountLinked
	if !errors.As(err, &linked) {
		t.Fatalf("want ErrAccountLinked, got %v", err)
	}
	if !slices.Equal(linked.Dependants, []string{"alpha", "zeta"}) {
		t.Errorf("dependants %v", linked.Dependants)
	}
	if got := cloud.Calls(); slices.Contains(got, "delete cloud") {
		t.Errorf("provider delete ran: %v", got)
	}
	overview, err := m.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := overview.Account("cloud"); !ok {
		t.Error("account was deleted")
	}
	plan, err := m.DeletePlan("cloud")
	if err != nil || !slices.Equal(plan.Dependants, []string{"alpha", "zeta"}) {
		t.Errorf("plan %+v err %v", plan, err)
	}
	if _, err := m.Delete(context.Background(), "alpha", manager.DeleteOptions{}); err != nil {
		t.Errorf("deleting a dependant must work: %v", err)
	}
	_ = fmt.Sprint
}
