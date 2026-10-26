package tui

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/k0wl0n/gctx/pkg/manager"
	"github.com/k0wl0n/gctx/pkg/provider"
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
	createReqs  []manager.CreateRequest
	updateReqs  []manager.UpdateRequest
	updateErr   error
	loginRes    manager.LoginResult
	loginErr    error
	loginFn     func(ctx context.Context, name string, io manager.LoginIO) (manager.LoginResult, error) // overrides loginRes and loginErr
	saveRes     manager.SaveResult
	saveErr     error
	deleteRes   manager.DeleteResult
	deleteErr   error
	deleteOpts  []manager.DeleteOptions
	deletePlans map[string]manager.DeletePlan // DeletePlan results by account; a default plan otherwise
	runCmd      *exec.Cmd
	runErr      error
	shellCmd    *exec.Cmd
	shellErr    error
	shellScopes []manager.ShellScope
	gcloudBin   string

	discoverRes manager.DiscoveryResult
	discoverErr error
	importRes   manager.ImportResult
	importErr   error
	imported    [][]manager.DiscoveredAccount
	onImport    func(items []manager.DiscoveredAccount) // lets a test change the overview an import produces
	cliVersions map[provider.ID]string

	envs            map[string][]string                      // Env results by account
	providerReports map[provider.ID]manager.CredentialReport // CheckProviderCredentials results

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
	if gate == nil {
		gate = f.gates[entry] // per-argument gate, e.g. "CheckProviderCredentials aws"
	}
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

func (f *fakeService) CheckProviderCredentials(ctx context.Context, id provider.ID) (manager.CredentialReport, error) {
	f.record("CheckProviderCredentials", string(id))
	return f.providerReports[id], f.reportErr
}

func (f *fakeService) Env(name string) ([]string, error) {
	f.record("Env", name)
	return f.envs[name], nil
}

func (f *fakeService) Providers() []manager.ProviderInfo {
	f.record("Providers")
	return f.overview.Providers
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

func (f *fakeService) CreateAccount(ctx context.Context, req manager.CreateRequest) (manager.CreateResult, error) {
	f.record("CreateAccount", req.Name)
	f.mu.Lock()
	f.createReqs = append(f.createReqs, req)
	f.mu.Unlock()
	return f.createRes, f.createErr
}

func (f *fakeService) UpdateAccount(ctx context.Context, name string, req manager.UpdateRequest) error {
	f.record("UpdateAccount", name)
	f.mu.Lock()
	f.updateReqs = append(f.updateReqs, req)
	f.mu.Unlock()
	return f.updateErr
}

func (f *fakeService) Login(ctx context.Context, name string, io manager.LoginIO) (manager.LoginResult, error) {
	f.record("Login", name)
	if f.loginFn != nil {
		return f.loginFn(ctx, name, io)
	}
	return f.loginRes, f.loginErr
}

func (f *fakeService) SaveCredentials(ctx context.Context, name string) (manager.SaveResult, error) {
	f.record("SaveCredentials", name)
	return f.saveRes, f.saveErr
}

func (f *fakeService) DeletePlan(name string) (manager.DeletePlan, error) {
	f.record("DeletePlan", name)
	if plan, ok := f.deletePlans[name]; ok {
		return plan, nil
	}
	return manager.DeletePlan{
		Items: []provider.DeleteItem{{Key: provider.DeleteKeyGcloudConfig, Label: "gcloud configuration " + name + "-config", Default: true}},
		Kept:  "the GCP project",
	}, nil
}

func (f *fakeService) Delete(ctx context.Context, name string, opt manager.DeleteOptions) (manager.DeleteResult, error) {
	f.record("Delete", name)
	f.mu.Lock()
	f.deleteOpts = append(f.deleteOpts, opt)
	f.mu.Unlock()
	return f.deleteRes, f.deleteErr
}

func (f *fakeService) RunCommand(name string, args []string) (*exec.Cmd, error) {
	f.record("RunCommand", name)
	return f.runCmd, f.runErr
}

func (f *fakeService) ShellCommand(name string, scope manager.ShellScope) (*exec.Cmd, error) {
	f.shellScopes = append(f.shellScopes, scope)
	f.record("ShellCommand", name)
	return f.shellCmd, f.shellErr
}

func (f *fakeService) GcloudBin() string { return f.gcloudBin }

func (f *fakeService) Discover() (manager.DiscoveryResult, error) {
	f.record("Discover")
	return f.discoverRes, f.discoverErr
}

func (f *fakeService) ImportAccounts(items []manager.DiscoveredAccount) (manager.ImportResult, error) {
	f.record("ImportAccounts")
	f.mu.Lock()
	f.imported = append(f.imported, items)
	hook := f.onImport
	f.mu.Unlock()
	if hook != nil {
		hook(items)
	}
	return f.importRes, f.importErr
}

func (f *fakeService) CLIVersions(ctx context.Context) map[provider.ID]string {
	f.record("CLIVersions")
	return f.cliVersions
}

// gcpInfo is the GCP provider's metadata as the registry reports it.
func gcpInfo(count int) manager.ProviderInfo {
	return manager.ProviderInfo{
		ID: provider.GCP, Label: "GCP", Category: provider.CategoryCloud, Slot: "gcp", TargetLabel: "PROJECT",
		Badge:        provider.Badge{Text: "GCP", Style: provider.BadgeFilled, Color: "#6ea8fe"},
		CLI:          provider.CLIStatus{Name: "gcloud", Bin: "gcloud", Found: true},
		CheckTimeout: 15 * time.Second, Count: count, CanLogin: true,
		Form: []provider.FormField{{Key: provider.FormTargetKey, Label: "Project ID", Validate: manager.ValidateProjectID}},
	}
}

// sampleOverview builds an overview with the named GCP accounts, all READY.
func sampleOverview(active string, names ...string) manager.Overview {
	overview := manager.Overview{
		ConfigPath:         "~/.config/gctx/config.json",
		ActiveAccount:      active,
		ActiveBySlot:       map[string]string{},
		GcloudActiveConfig: active + "-config",
		DefaultADC:         manager.ADCActive,
		GcloudAvailable:    true,
		Providers:          []manager.ProviderInfo{gcpInfo(len(names))},
	}
	if active != "" {
		overview.ActiveBySlot["gcp"] = active
	}
	for _, name := range names {
		overview.Accounts = append(overview.Accounts, manager.AccountStatus{
			Name:               name,
			Provider:           provider.GCP,
			Category:           provider.CategoryCloud,
			TargetLabel:        "PROJECT",
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

// fixtureNow is the clock of the multi-provider fixture.
var fixtureNow = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)

// The AI providers below do not exist as real providers yet: the fixture
// fabricates their metadata so the TUI can be shown rendering it.
const (
	fakeClaude     provider.ID = "claude-code"
	fakeCodex      provider.ID = "codex"
	fakeSumoPod    provider.ID = "sumopod"
	fakeOpenRouter provider.ID = "openrouter"
)

func settingColumn(key, header string, width int) provider.Column {
	return provider.Column{Key: key, Header: header, Width: width, Value: func(v provider.AccountView) string { return v.Settings[key] }}
}

// multiProviders is the canvas registry: three clouds, two AI tools and two
// gateways sharing the openai-api slot.
func multiProviders() []manager.ProviderInfo {
	info := func(id provider.ID, label string, cat provider.Category, slot, target string, badge provider.Badge, cols ...provider.Column) manager.ProviderInfo {
		return manager.ProviderInfo{
			ID: id, Label: label, Category: cat, Slot: slot, TargetLabel: target, Badge: badge, Columns: cols,
			CLI: provider.CLIStatus{Bin: "/usr/bin/" + string(id), Found: true}, CheckTimeout: 8 * time.Second, CanLogin: true,
		}
	}
	infos := []manager.ProviderInfo{
		info(provider.GCP, "GCP", provider.CategoryCloud, "gcp", "PROJECT", provider.Badge{Text: "GCP", Style: provider.BadgeFilled, Color: "#6ea8fe"}),
		info(provider.AWS, "AWS", provider.CategoryCloud, "aws", "PROFILE", provider.Badge{Text: "AWS", Style: provider.BadgeFilled, Color: "#ffa657"},
			settingColumn("region", "REGION", 14)),
		info(provider.Azure, "Azure", provider.CategoryCloud, "azure", "SUBSCRIPTION", provider.Badge{Text: "AZR", Style: provider.BadgeFilled, Color: "#56d4dd"},
			settingColumn("tenant", "TENANT", 36)),
		info(fakeClaude, "Claude Code", provider.CategoryAITool, "claude", "ORG", provider.Badge{Text: "CLD", Style: provider.BadgeOutline, Color: "#e3b38c"},
			settingColumn("auth", "AUTH", 12)),
		info(fakeCodex, "Codex", provider.CategoryAITool, "codex", "WORKSPACE", provider.Badge{Text: "OAI", Style: provider.BadgeOutline, Color: "#b9c7d6"}),
		info(fakeSumoPod, "SumoPod", provider.CategoryAIGateway, "openai-api", "BASE URL", provider.Badge{Text: "SMP", Style: provider.BadgeDashed, Color: "#e8a6c8"},
			settingColumn("key_ref", "KEY REF", 30)),
		info(fakeOpenRouter, "OpenRouter", provider.CategoryAIGateway, "openai-api", "BASE URL", provider.Badge{Text: "ORT", Style: provider.BadgeDashed, Color: "#a3b1ff"},
			settingColumn("key_ref", "KEY REF", 30)),
	}
	for i := range infos {
		infos[i].EnvBased = infos[i].ID != provider.GCP    // GCP switches globally
		infos[i].NetworkAuth = infos[i].ID != provider.GCP // GCP checks with a local gcloud call
	}
	return infos
}

// multiOverview is the canvas data: 9 GCP, 2 AWS, 1 Azure, 3 Claude Code,
// 1 Codex, 2 SumoPod and 1 OpenRouter accounts. Sample data only.
func multiOverview() manager.Overview {
	infos := multiProviders()
	byID := map[provider.ID]manager.ProviderInfo{}
	for _, info := range infos {
		byID[info.ID] = info
	}
	checked := fixtureNow.Add(-14 * time.Second)
	ready := func(id provider.ID, name, target, identity string, expiresIn time.Duration) manager.AccountStatus {
		info := byID[id]
		a := manager.AccountStatus{
			Name: name, Provider: id, Category: info.Category, TargetLabel: info.TargetLabel,
			ProjectID: target, Identity: identity, Status: manager.HealthReady, Checked: checked,
		}
		if expiresIn != 0 {
			a.ExpiresAt = fixtureNow.Add(expiresIn)
		}
		return a
	}
	gcp := func(name, project, email string, expiresIn time.Duration) manager.AccountStatus {
		a := ready(provider.GCP, name, project, email, expiresIn)
		a.ConfigName, a.Email, a.ADCPath, a.ADCExists, a.GcloudConfigExists = name, email, "/adc/"+name+".json", true, true
		a.Details = []provider.Field{{Label: "gcloud config", Value: name}}
		return a
	}
	withStatus := func(a manager.AccountStatus, h manager.Health) manager.AccountStatus {
		a.Status = h
		a.Findings = []manager.Finding{{Health: h}}
		return a
	}
	withSettings := func(a manager.AccountStatus, kv ...string) manager.AccountStatus {
		a.Settings = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			a.Settings[kv[i]] = kv[i+1]
		}
		return a
	}
	withBudget := func(a manager.AccountStatus, spent, limit float64) manager.AccountStatus {
		a.Budget = &provider.Budget{Spent: spent, Limit: limit, Currency: "USD"}
		return a
	}

	sbx := gcp("sbx-aliz", "sandbox-dev-anggana", "dev@aliz.example", 0)
	sbx.ADCExists = false
	expired := gcp("byoip-lab", "byoip-lab-eu-west", "ops@emarsys.example", -48*time.Hour)

	prod := withSettings(ready(provider.AWS, "prod-aws", "prod-sso", "arn:aws:sts::123456789012:assumed-role/PlatformDev/dev", 3*time.Hour+12*time.Minute),
		"region", "eu-west-1", "auth", "sso")
	prod.Details = []provider.Field{{Label: "Profile", Value: "prod-sso"}, {Label: "Region", Value: "eu-west-1"}, {Label: "Auth", Value: "sso"}}
	legacy := withSettings(withStatus(ready(provider.AWS, "legacy-aws", "legacy", "", 0), manager.HealthAuthRequired), "region", "us-east-1")
	legacy.Checked = checked
	azure := withSettings(ready(provider.Azure, "corp-azure", "8c1f0a3e-0000-4000-8000-00000000e2a9", "dev@corp.example", 0), "tenant", "corp.example")
	azure.Checked = time.Time{} // never checked: shows "checking…" while its check runs

	accounts := []manager.AccountStatus{
		gcp("cloudsuite", "sap-gap-cloud-asap-s", "ops@emarsys.example", 52*time.Minute),
		gcp("infra-hub-prd", "emarsys-infra-hub-prd", "ops@emarsys.example", 52*time.Minute),
		gcp("infra-hub-stg", "emarsys-infra-hub-stg", "ops@emarsys.example", 52*time.Minute),
		gcp("gap-dev", "gap-platform-dev", "ops@emarsys.example", 52*time.Minute),
		gcp("hcp-agents", "hcp-tf-agents-poc", "dev@aliz.example", 7*time.Hour),
		gcp("rca-poc", "k8s-ai-rca-poc", "dev@aliz.example", 7*time.Hour),
		gcp("demo-ai", "ai-builder-cup-demo", "dev@aliz.example", 7*time.Hour),
		withStatus(sbx, manager.HealthAuthRequired),
		withStatus(expired, manager.HealthExpired),
		prod, legacy, azure,
		withSettings(ready(fakeClaude, "claude-work", "org Aliz Tech", "dev@aliz.example", 6*24*time.Hour), "auth", "subscription"),
		withSettings(ready(fakeClaude, "claude-vertex", "via cloudsuite", "Vertex AI · us-east5", 52*time.Minute), "auth", "vertex"),
		withSettings(withStatus(ready(fakeClaude, "claude-personal", "personal", "", 0), manager.HealthAuthRequired), "auth", "subscription"),
		ready(fakeCodex, "codex-work", "workspace Aliz", "dev@aliz.example", 0),
		withSettings(withBudget(ready(fakeSumoPod, "sumopod-dev", "ai.sumopod.com", "", 0), 1.85, 5), "key_ref", "op://Private/sumopod-dev"),
		withSettings(withBudget(withStatus(ready(fakeSumoPod, "sumopod-prod", "ai.sumopod.com", "", 0), manager.HealthBudgetLow), 18.40, 20), "key_ref", "gcpsm://aliz-ai/sumopod-prod"),
		withSettings(withBudget(ready(fakeOpenRouter, "openrouter", "openrouter.ai", "", 0), 2.60, 10), "key_ref", "keychain:openrouter"),
	}
	active := map[string]string{"gcp": "cloudsuite", "aws": "prod-aws", "claude": "claude-work", "codex": "codex-work", "openai-api": "sumopod-dev"}
	slotOf := map[provider.ID]string{}
	for _, info := range infos {
		slotOf[info.ID] = info.Slot
	}
	for i, a := range accounts {
		accounts[i].Active = active[slotOf[a.Provider]] == a.Name
		info := byID[a.Provider]
		info.Count++
		byID[a.Provider] = info
	}
	for i := range infos {
		infos[i].Count = byID[infos[i].ID].Count
	}
	return manager.Overview{
		ConfigPath:         "~/.config/gctx/config.json",
		Accounts:           accounts,
		ActiveAccount:      "cloudsuite",
		ActiveBySlot:       active,
		GcloudActiveConfig: "cloudsuite",
		DefaultADC:         manager.ADCActive,
		GcloudAvailable:    true,
		Providers:          infos,
	}
}

// multiEnvs is what Env reports for the fixture; the gateway key is a
// reference, never a value.
func multiEnvs() map[string][]string {
	return map[string][]string{
		"cloudsuite":  {"CLOUDSDK_ACTIVE_CONFIG_NAME=cloudsuite"},
		"prod-aws":    {"AWS_PROFILE=prod-sso"},
		"legacy-aws":  {"AWS_PROFILE=legacy"},
		"corp-azure":  {"AZURE_CONFIG_DIR=~/.config/gctx/azure/corp-azure"},
		"claude-work": {"CLAUDE_CONFIG_DIR=~/.config/gctx/ai/claude-work"},
		"codex-work":  {"CODEX_HOME=~/.config/gctx/ai/codex-work"},
		"sumopod-dev": {"OPENAI_BASE_URL=https://ai.sumopod.com/v1", "OPENAI_API_KEY=op://Private/sumopod-dev/credential"},
	}
}

// multiReports is the outcome of every provider's check.
func multiReports() map[provider.ID]manager.CredentialReport {
	return map[provider.ID]manager.CredentialReport{
		provider.GCP:   {Accounts: []string{"cloudsuite", "infra-hub-prd", "infra-hub-stg", "gap-dev", "hcp-agents", "rca-poc", "demo-ai", "byoip-lab"}},
		provider.AWS:   {Accounts: []string{"prod-aws"}},
		provider.Azure: {Accounts: []string{"corp-azure"}},
		fakeClaude:     {Accounts: []string{"claude-work", "claude-vertex"}},
		fakeCodex:      {Accounts: []string{"codex-work"}},
		fakeSumoPod:    {Accounts: []string{"sumopod-dev", "sumopod-prod"}},
		fakeOpenRouter: {Accounts: []string{"openrouter"}},
	}
}

func newMultiService() *fakeService {
	svc := newFakeService(multiOverview())
	svc.envs = multiEnvs()
	svc.providerReports = multiReports()
	return svc
}
