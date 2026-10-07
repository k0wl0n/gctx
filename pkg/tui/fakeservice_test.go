package tui

import (
	"context"
	"os/exec"
	"sync"

	"github.com/k0wl0n/gctx/pkg/manager"
)

// fakeService is an in-memory Service with scripted results, recorded calls
// and an optional gate that blocks a method until released.
type fakeService struct {
	mu    sync.Mutex
	calls []string

	overview    manager.Overview
	overviewErr error
	report      manager.CredentialReport
	reportErr   error
	switchRes   manager.SwitchResult
	switchErr   error
	createRes   manager.CreateResult
	createErr   error
	loginRes    manager.LoginResult
	loginErr    error
	saveRes     manager.SaveResult
	saveErr     error
	deleteRes   manager.DeleteResult
	deleteErr   error
	runCmd      *exec.Cmd
	runErr      error
	shellCmd    *exec.Cmd
	shellErr    error
	gcloudBin   string

	// gates maps a method name ("Switch", "CheckCredentials", ...) to a channel
	// that must be closed (or sent to) before the method returns.
	gates map[string]chan struct{}
}

func newFakeService(overview manager.Overview) *fakeService {
	return &fakeService{overview: overview, gcloudBin: "gcloud", gates: map[string]chan struct{}{}}
}

// block makes method wait until the returned release function is called.
func (f *fakeService) block(method string) (release func()) {
	gate := make(chan struct{})
	f.mu.Lock()
	f.gates[method] = gate
	f.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

func (f *fakeService) record(method string, args ...string) {
	f.mu.Lock()
	entry := method
	for _, a := range args {
		entry += " " + a
	}
	f.calls = append(f.calls, entry)
	gate := f.gates[method]
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
}

// Calls returns the recorded calls in order.
func (f *fakeService) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeService) Overview() (manager.Overview, error) {
	f.record("Overview")
	return f.overview, f.overviewErr
}

func (f *fakeService) CheckCredentials(ctx context.Context) (manager.CredentialReport, error) {
	f.record("CheckCredentials")
	return f.report, f.reportErr
}

func (f *fakeService) Info(name string) (manager.AccountStatus, error) {
	f.record("Info", name)
	account, _ := f.overview.Account(name)
	return account, nil
}

func (f *fakeService) Switch(ctx context.Context, name string) (manager.SwitchResult, error) {
	f.record("Switch", name)
	return f.switchRes, f.switchErr
}

func (f *fakeService) Create(ctx context.Context, name, projectID string) (manager.CreateResult, error) {
	f.record("Create", name, projectID)
	return f.createRes, f.createErr
}

func (f *fakeService) Login(ctx context.Context, name string, io manager.LoginIO) (manager.LoginResult, error) {
	f.record("Login", name)
	return f.loginRes, f.loginErr
}

func (f *fakeService) SaveCredentials(ctx context.Context, name string) (manager.SaveResult, error) {
	f.record("SaveCredentials", name)
	return f.saveRes, f.saveErr
}

func (f *fakeService) Delete(ctx context.Context, name string, opt manager.DeleteOptions) (manager.DeleteResult, error) {
	f.record("Delete", name)
	return f.deleteRes, f.deleteErr
}

func (f *fakeService) RunCommand(name string, args []string) (*exec.Cmd, error) {
	f.record("RunCommand", name)
	return f.runCmd, f.runErr
}

func (f *fakeService) ShellCommand(name string) (*exec.Cmd, error) {
	f.record("ShellCommand", name)
	return f.shellCmd, f.shellErr
}

func (f *fakeService) GcloudBin() string { return f.gcloudBin }

// sampleOverview builds an overview with the named accounts, all READY.
func sampleOverview(active string, names ...string) manager.Overview {
	overview := manager.Overview{
		ConfigPath:         "/tmp/gctx/config.json",
		ActiveAccount:      active,
		GcloudActiveConfig: active + "-config",
		DefaultADC:         manager.ADCActive,
		GcloudAvailable:    true,
	}
	for _, name := range names {
		overview.Accounts = append(overview.Accounts, manager.AccountStatus{
			Name:               name,
			ConfigName:         name + "-config",
			ProjectID:          name + "-project",
			Email:              name + "@example.com",
			Active:             name == active,
			ADCExists:          true,
			GcloudConfigExists: true,
			Status:             manager.HealthReady,
		})
	}
	return overview
}
