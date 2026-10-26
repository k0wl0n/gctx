package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
)

const (
	testDeviceURL  = "https://microsoft.com/devicelogin"
	testDeviceCode = "F7K2-Q9XM"
)

// loginPump runs the commands the model returns on goroutines and feeds every
// message back, so the streaming login can be driven without a runtime.
type loginPump struct {
	t       *testing.T
	m       Model
	pending []chan tea.Msg
}

func newLoginPump(t *testing.T, svc Service) *loginPump {
	t.Helper()
	m := newTestModel(t, svc, 120, 30)
	m.loginTick = func(int) tea.Cmd { return nil }
	m.exec = execSync("\n", nil)
	m.openURL = func(string) error { return errors.New("browser launched by a test") }
	p := &loginPump{t: t, m: m}
	t.Cleanup(func() {
		if p.m.login != nil {
			p.m.login.cancel()
		}
	})
	return p
}

func (p *loginPump) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	p.pending = append(p.pending, ch)
}

func (p *loginPump) deliver(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, cmd := range msg {
			p.run(cmd)
		}
	default:
		var cmd tea.Cmd
		p.m, cmd = p.m.dispatch(msg)
		p.run(cmd)
	}
}

// settle delivers messages until none arrives for a short while.
func (p *loginPump) settle() {
	p.t.Helper()
	quiet := time.Now()
	for time.Since(quiet) < 150*time.Millisecond {
		for i := 0; i < len(p.pending); i++ {
			select {
			case msg := <-p.pending[i]:
				p.pending = slices.Delete(p.pending, i, i+1)
				i--
				p.deliver(msg)
				quiet = time.Now()
			default:
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (p *loginPump) key(names ...string) {
	p.t.Helper()
	for _, name := range names {
		var cmd tea.Cmd
		p.m, cmd = p.m.dispatch(keyMsg(name))
		p.run(cmd)
		p.settle()
	}
}

func (p *loginPump) screen() string { return screen(p.m) }

func emit(lio manager.LoginIO, events ...provider.LoginEvent) {
	for _, e := range events {
		lio.Progress(e)
	}
}

func step(kind provider.LoginEventKind, label string) provider.LoginEvent {
	return provider.LoginEvent{Kind: kind, Step: label}
}

func output(line string) provider.LoginEvent {
	return provider.LoginEvent{Kind: provider.LoginOutput, Output: line}
}

var deviceCodeEvent = provider.LoginEvent{Kind: provider.LoginDeviceCode, URL: testDeviceURL, Code: testDeviceCode}

const (
	stepIsolateLabel = "isolate config"
	stepLoginLabel   = "az login --use-device-code"
)

// loginFixture is the multi-provider canvas with a streaming login provider
// whose account corp-azure needs login.
func loginFixture(t *testing.T) *fakeService {
	t.Helper()
	svc := newMultiService()
	for i, info := range svc.overview.Providers {
		if info.ID == provider.Azure {
			svc.overview.Providers[i].StreamsLogin = true
			svc.overview.Providers[i].CLI.Name = "az"
		}
	}
	setHealth(svc, "corp-azure", manager.HealthAuthRequired)
	return svc
}

func setHealth(svc *fakeService, name string, h manager.Health) {
	for i, a := range svc.overview.Accounts {
		if a.Name == name {
			svc.overview.Accounts[i].Status = h
			svc.overview.Accounts[i].Findings = []manager.Finding{{Health: h}}
		}
	}
}

func addAccountLike(svc *fakeService, template, name string, h manager.Health) {
	for _, a := range svc.overview.Accounts {
		if a.Name == template {
			a.Name, a.Active = name, false
			svc.overview.Accounts = append(svc.overview.Accounts, a)
		}
	}
	setHealth(svc, name, h)
}

// onlyStale makes every account but the named ones READY.
func onlyStale(svc *fakeService, names ...string) {
	for i, a := range svc.overview.Accounts {
		if !slices.Contains(names, a.Name) {
			svc.overview.Accounts[i].Status = manager.HealthReady
			svc.overview.Accounts[i].Findings = nil
		}
	}
}

func selectAccount(p *loginPump, name string) {
	p.t.Helper()
	p.m.selected = name
}

// blockedLogin emits events then waits for the context, like az waiting for the user.
func blockedLogin(killed chan<- string, events ...provider.LoginEvent) func(context.Context, string, manager.LoginIO) (manager.LoginResult, error) {
	return func(ctx context.Context, name string, lio manager.LoginIO) (manager.LoginResult, error) {
		emit(lio, events...)
		<-ctx.Done()
		if killed != nil {
			killed <- name
		}
		return manager.LoginResult{Account: name}, ctx.Err()
	}
}

func TestLoginShowsDeviceCode(t *testing.T) {
	svc := loginFixture(t)
	svc.loginFn = blockedLogin(nil,
		step(provider.LoginStepPending, stepIsolateLabel), step(provider.LoginStepPending, stepLoginLabel),
		step(provider.LoginStepStarted, stepIsolateLabel), step(provider.LoginStepDone, stepIsolateLabel),
		step(provider.LoginStepStarted, stepLoginLabel), deviceCodeEvent)
	p := newLoginPump(t, svc)
	selectAccount(p, "corp-azure")
	p.key("l")

	if p.m.screen != screenLogin {
		t.Fatalf("screen %v, want login", p.m.screen)
	}
	out := p.screen()
	for _, want := range []string{
		"accounts ›", " AZR ", "corp-azure › login", "✓ isolate config", "az login --use-device-code",
		testDeviceURL, "F 7 K 2 - Q 9 X M", "open browser", "copy code",
		"az output", "v full log", "cancel (stops az, account unchanged)",
		"tokens stay in the isolated az dir;", "reads them", "on success: status → READY, back to list",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("screen lacks %q:\n%s", want, out)
		}
	}
}

func TestLoginStreamsOutput(t *testing.T) {
	svc := loginFixture(t)
	svc.loginFn = blockedLogin(nil, output("$ az login --use-device-code"), output("first line"), output("second line"))
	p := newLoginPump(t, svc)
	selectAccount(p, "corp-azure")
	p.key("l")

	out := p.screen()
	for _, want := range []string{"$ az login --use-device-code", "first line", "second line"} {
		if !strings.Contains(out, want) {
			t.Errorf("output pane lacks %q:\n%s", want, out)
		}
	}
	p.key("v")
	out = p.screen()
	if !strings.Contains(out, "full log") || !strings.Contains(out, "second line") {
		t.Errorf("full log view:\n%s", out)
	}
	if strings.Contains(out, "open browser") {
		t.Errorf("full log still shows the steps column:\n%s", out)
	}
	p.key("v")
	if p.m.login.fullLog || !strings.Contains(p.screen(), "v full log") {
		t.Errorf("second v did not return to the split view:\n%s", p.screen())
	}
}

func TestLoginCancelKillsChildAndKeepsAccount(t *testing.T) {
	svc := loginFixture(t)
	killed := make(chan string, 1)
	svc.loginFn = blockedLogin(killed, deviceCodeEvent)
	p := newLoginPump(t, svc)
	selectAccount(p, "corp-azure")
	p.key("l")
	p.key("esc")

	select {
	case name := <-killed:
		if name != "corp-azure" {
			t.Errorf("killed %q", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("login context was not cancelled")
	}
	p.settle()
	if p.m.screen != screenMain || p.m.login != nil {
		t.Fatalf("screen %v login %v, want the list", p.m.screen, p.m.login)
	}
	if got, _ := p.m.overview.Account("corp-azure"); got.Status != manager.HealthAuthRequired {
		t.Errorf("status %s, want AUTH REQUIRED unchanged", got.Status)
	}
	if !strings.Contains(p.m.status.text, "cancelled") || !strings.Contains(p.m.status.text, "unchanged") {
		t.Errorf("status %q", p.m.status.text)
	}
}

func TestLoginQueueOrderAndDedupe(t *testing.T) {
	svc := loginFixture(t)
	setHealth(svc, "claude-vertex", manager.HealthExpired)
	setHealth(svc, "claude-personal", manager.HealthAuthRequired)
	for i, a := range svc.overview.Accounts {
		if a.Name == "claude-vertex" || a.Name == "claude-personal" {
			svc.overview.Accounts[i].Settings["via"] = "cloudsuite"
		}
	}
	m := newTestModel(t, svc, 120, 30)
	m.exec = func(tea.ExecCommand, tea.ExecCallback) tea.Cmd { return nil }
	m.tab = 2 // the queue ignores the tab and the filter
	m.filter.query = "no-such-account"
	m.refilter()

	m, _ = m.dispatch(keyMsg("L"))
	if m.queue == nil {
		t.Fatal("no queue after L")
	}
	want := []string{"sbx-aliz", "byoip-lab", "legacy-aws", "corp-azure", "cloudsuite"}
	if !slices.Equal(m.queue.entries, want) {
		t.Errorf("queue %v, want %v (list order, linked rows once as their cloud account)", m.queue.entries, want)
	}
}

func TestLoginQueueNothingStale(t *testing.T) {
	svc := loginFixture(t)
	onlyStale(svc)
	m := newTestModel(t, svc, 120, 30)
	m, _ = m.dispatch(keyMsg("L"))
	if m.queue != nil || !strings.Contains(m.status.text, "no account needs login") {
		t.Errorf("queue %v status %q", m.queue, m.status.text)
	}
}

func TestLoginQueueSkip(t *testing.T) {
	svc := loginFixture(t)
	addAccountLike(svc, "corp-azure", "second-azure", manager.HealthAuthRequired)
	onlyStale(svc, "corp-azure", "second-azure")
	killed := make(chan string, 2)
	svc.loginFn = func(ctx context.Context, name string, lio manager.LoginIO) (manager.LoginResult, error) {
		if name == "corp-azure" {
			return blockedLogin(killed, deviceCodeEvent)(ctx, name, lio)
		}
		return manager.LoginResult{Account: name}, nil
	}
	p := newLoginPump(t, svc)
	p.key("L")

	out := p.screen()
	if !strings.Contains(out, "queue 1 of 2 · next second-azure") {
		t.Errorf("queue header missing:\n%s", out)
	}
	p.key("n")

	if name := <-killed; name != "corp-azure" {
		t.Errorf("skipped login killed %q", name)
	}
	if p.m.screen != screenMain || p.m.queue != nil {
		t.Fatalf("screen %v queue %v, want the list", p.m.screen, p.m.queue)
	}
	if got := svc.Calls(); !slices.Contains(got, "Login corp-azure") || !slices.Contains(got, "Login second-azure") {
		t.Errorf("calls %v", got)
	}
	if p.m.status.text != "logged in 1 of 2, 1 skipped" {
		t.Errorf("summary %q", p.m.status.text)
	}
}

func TestLoginQueueMixesModes(t *testing.T) {
	svc := loginFixture(t)
	onlyStale(svc, "sbx-aliz", "corp-azure")
	p := newLoginPump(t, svc)
	p.key("L") // sbx-aliz logs in through the terminal, corp-azure through the screen

	if got := svc.Calls(); !slices.Contains(got, "Login sbx-aliz") || !slices.Contains(got, "Login corp-azure") {
		t.Errorf("calls %v", got)
	}
	if p.m.screen != screenMain || p.m.status.text != "logged in 2 of 2" {
		t.Errorf("screen %v status %q", p.m.screen, p.m.status.text)
	}
}

func TestLoginSuccessRefreshesRow(t *testing.T) {
	svc := loginFixture(t)
	svc.loginFn = func(ctx context.Context, name string, lio manager.LoginIO) (manager.LoginResult, error) {
		emit(lio, deviceCodeEvent)
		setHealth(svc, name, manager.HealthReady)
		return manager.LoginResult{Account: name}, nil
	}
	p := newLoginPump(t, svc)
	selectAccount(p, "corp-azure")
	p.key("l")

	if p.m.screen != screenMain {
		t.Fatalf("screen %v, want the list after success", p.m.screen)
	}
	if got, _ := p.m.overview.Account("corp-azure"); got.Status != manager.HealthReady {
		t.Errorf("row status %s, want READY", got.Status)
	}
	if p.m.selected != "corp-azure" || p.m.status.text != "logged in corp-azure" {
		t.Errorf("selected %q status %q", p.m.selected, p.m.status.text)
	}
	if !slices.Contains(svc.Calls(), "CheckProviderCredentials azure") {
		t.Errorf("credentials were not re-checked: %v", svc.Calls())
	}
}

func TestLoginFailureStaysOnScreen(t *testing.T) {
	svc := loginFixture(t)
	svc.loginFn = func(ctx context.Context, name string, lio manager.LoginIO) (manager.LoginResult, error) {
		emit(lio, step(provider.LoginStepPending, stepLoginLabel), step(provider.LoginStepStarted, stepLoginLabel),
			output("ERROR: AADSTS70016: authorization declined"), step(provider.LoginStepFailed, stepLoginLabel))
		return manager.LoginResult{Account: name}, errors.New("az login failed: exit status 1")
	}
	p := newLoginPump(t, svc)
	selectAccount(p, "corp-azure")
	p.key("l")

	if p.m.screen != screenLogin {
		t.Fatalf("screen %v, want the login screen to stay", p.m.screen)
	}
	out := p.screen()
	for _, want := range []string{"✗ az login --use-device-code", "AADSTS70016", "az login failed: exit status 1", "back to list"} {
		if !strings.Contains(out, want) {
			t.Errorf("failure screen lacks %q:\n%s", want, out)
		}
	}
	p.key("esc")
	if p.m.screen != screenMain || p.m.login != nil {
		t.Errorf("esc did not return to the list: screen %v", p.m.screen)
	}
	if got, _ := p.m.overview.Account("corp-azure"); got.Status != manager.HealthAuthRequired {
		t.Errorf("status %s after failure", got.Status)
	}
}

func TestOpenBrowserUsesInjectedFunc(t *testing.T) {
	svc := loginFixture(t)
	svc.loginFn = blockedLogin(nil, deviceCodeEvent)
	p := newLoginPump(t, svc)
	var opened []string
	p.m.openURL = func(url string) error { opened = append(opened, url); return nil }
	selectAccount(p, "corp-azure")
	p.key("l")
	p.key("o")

	if len(opened) != 1 || opened[0] != testDeviceURL {
		t.Errorf("opened %v, want [%s]", opened, testDeviceURL)
	}
}

func TestOpenBrowserRefusesNonHTTPSURL(t *testing.T) {
	svc := loginFixture(t)
	svc.loginFn = blockedLogin(nil, provider.LoginEvent{Kind: provider.LoginDeviceCode, URL: "file:///etc/passwd", Code: "X"})
	p := newLoginPump(t, svc)
	opened := false
	p.m.openURL = func(string) error { opened = true; return nil }
	selectAccount(p, "corp-azure")
	p.key("l")
	p.key("o")
	if opened {
		t.Error("a non-https URL reached the opener")
	}
}

func TestCopyCodeEmitsClipboardCmd(t *testing.T) {
	svc := loginFixture(t)
	svc.loginFn = blockedLogin(nil, deviceCodeEvent)
	p := newLoginPump(t, svc)
	selectAccount(p, "corp-azure")
	p.key("l")

	var cmd tea.Cmd
	p.m, cmd = p.m.dispatch(keyMsg("y"))
	if !hasMessage(cmd, tea.SetClipboard(testDeviceCode)()) {
		t.Error("y did not emit the clipboard command for the code")
	}
	if p.m.status.text != "copied code" {
		t.Errorf("status %q", p.m.status.text)
	}
}

func hasMessage(cmd tea.Cmd, want tea.Msg) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			if hasMessage(c, want) {
				return true
			}
		}
	default:
		return msg == want
	}
	return false
}

func TestLoginScreenLayoutAt120And70(t *testing.T) {
	for _, size := range []struct{ w, h int }{{120, 30}, {70, 24}, {60, 16}} {
		svc := loginFixture(t)
		addAccountLike(svc, "corp-azure", "second-azure", manager.HealthAuthRequired)
		onlyStale(svc, "corp-azure", "second-azure")
		long := strings.Repeat("a very long output line that has to wrap or truncate ", 4)
		svc.loginFn = blockedLogin(nil,
			step(provider.LoginStepPending, stepIsolateLabel), step(provider.LoginStepPending, stepLoginLabel),
			step(provider.LoginStepPending, "az account set --subscription 8c1f0a3e-0000-4000-8000-00000000e2a9"),
			step(provider.LoginStepDone, stepIsolateLabel), step(provider.LoginStepStarted, stepLoginLabel),
			output(long), deviceCodeEvent)
		m := newTestModel(t, svc, size.w, size.h)
		m.loginTick = func(int) tea.Cmd { return nil }
		p := &loginPump{t: t, m: m}
		t.Cleanup(func() {
			if p.m.login != nil {
				p.m.login.cancel()
			}
		})
		selectAccount(p, "corp-azure")
		p.key("L")
		if p.m.screen != screenLogin {
			t.Fatalf("%dx%d: screen %v", size.w, size.h, p.m.screen)
		}
		lines := strings.Split(p.screen(), "\n")
		if len(lines) > size.h {
			t.Errorf("%dx%d: %d lines", size.w, size.h, len(lines))
		}
		for i, line := range lines {
			if w := ansi.StringWidth(line); w > size.w {
				t.Errorf("%dx%d: line %d is %d cells wide: %q", size.w, size.h, i, w, line)
			}
		}
		out := p.screen()
		for _, want := range []string{"corp-azure", "login", testDeviceURL, "F 7 K 2 - Q 9 X M", "cancel (stops"} {
			if size.w >= 70 && !strings.Contains(out, want) {
				t.Errorf("%dx%d lacks %q:\n%s", size.w, size.h, want, out)
			}
		}
		if size.w == 120 && !strings.Contains(out, "queue 1 of 2 · next second-azure") {
			t.Errorf("%dx%d lacks the queue header:\n%s", size.w, size.h, out)
		}
		if p.m.login != nil {
			p.m.login.cancel()
		}
	}
}

func TestBrowserCommand(t *testing.T) {
	for goos, want := range map[string]string{
		"darwin":  "open",
		"linux":   "xdg-open",
		"freebsd": "xdg-open",
		"windows": "rundll32 url.dll,FileProtocolHandler",
	} {
		name, args := browserCommand(goos, testDeviceURL)
		if got := strings.Join(append([]string{name}, args[:len(args)-1]...), " "); got != want || args[len(args)-1] != testDeviceURL {
			t.Errorf("%s: %q %v", goos, name, args)
		}
	}
}
