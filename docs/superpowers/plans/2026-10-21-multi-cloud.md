# Multi-Cloud Context Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Manage GCP, AWS, Azure and Tencent Cloud accounts from one CLI and TUI, then rename the tool.

**Architecture:** Introduce `pkg/provider.Provider`. GCP logic moves behind it with no behavior change; `manager.Manager` dispatches on `Account.Provider`. AWS, Azure and Tencent are env-based providers added one at a time. The TUI gains a cloud column, cloud tabs and a provider-aware create form. The rename is last.

**Tech Stack:** Go 1.26, cobra, bubbletea v2 / bubbles v2 / lipgloss v2 (all already in `go.mod`). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-21-multi-cloud-design.md`

## Global Constraints

- Work on a new branch `feat/multi-cloud`; one Conventional Commit per task, lowercase imperative subjects.
- No new Go dependencies. Pin nothing new; keep `go.sum` unchanged unless a task says otherwise.
- `config.json` stays backward compatible: a file with no `provider` fields loads as all-`gcp`, and the JSON key `project_id` is kept for every provider (accessor `Account.Target()`).
- Account names are unique across all providers (spec D2).
- gctx never stores, logs or prints secrets or tokens (spec D5). Debug logs stay redacted.
- `Overview()` stays file-reads only: no subprocess, no network. Network checks live in `CheckCredentials` (spec D6).
- A missing provider CLI must only affect that provider's rows, never `list`, the TUI, or other providers.
- Every task updates the docs it touches (`README.md`, `readthedocs/docs`); Task 10 does the final sweep.
- Tests use a fake CLI binary following the `internal/testutil/fakegcloud` pattern; no test calls a real cloud.

## Review Focus

1. A v1 `config.json` (no `provider`, no `settings`) loads, upgrades in memory, and round-trips without losing fields (Task 1).
2. Creating a second account whose name already exists under another provider is rejected with `ErrAccountExists` (Task 5).
3. `aws`/`az`/`tccli` not installed: `list` and the TUI still render all other accounts, the affected rows show `CLI MISSING` (Task 4).
4. A provider auth check that hangs (offline `sts`) times out per provider and does not delay other providers' results or block the TUI (Task 4).
5. Switching to an env-based account must never claim the cloud context changed; the result carries the exact apply instruction (Tasks 4, 6).

---

### Task 1: Config schema: provider and settings

**Files:**
- Modify: `pkg/config/config.go` (Account struct, `AddAccount`)
- Test: `pkg/config/config_test.go`

**Interfaces:**
- Produces: `Account.Provider string` (`json:"provider,omitempty"`), `Account.Settings map[string]string` (`json:"settings,omitempty"`), `func (a Account) Target() string` (returns `ProjectID`), `func (a Account) ProviderID() string` (returns `"gcp"` when `Provider == ""`).

- [ ] **Step 1: Write failing tests** in `pkg/config/config_test.go`: `TestLoadV1ConfigDefaultsToGCP` (file without `provider` → `ProviderID() == "gcp"`), `TestRoundTripPreservesSettings` (an AWS account with `Settings{"region":"eu-west-1"}` survives Save/Load), `TestTargetReturnsProjectID`.
- [ ] **Step 2: Run** `go test ./pkg/config/ -run 'V1|RoundTrip|Target' -v`. Expected: FAIL (fields undefined).
- [ ] **Step 3: Add the fields and accessors** to `Account`. No schema version number; absence of `provider` is the v1 marker.
- [ ] **Step 4: Run** `go test ./pkg/config/ -v`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(config): add provider and settings to account`.

### Task 2: Provider package, contract and registry

**Files:**
- Create: `pkg/provider/provider.go` (interface, ID constants, shared types), `pkg/provider/registry.go`, `pkg/provider/fake_test.go`
- Test: `pkg/provider/registry_test.go`

**Interfaces:**
- Consumes: `config.Account` from Task 1.
- Produces (exact):
  - `type ID string`; consts `GCP ID = "gcp"`, `AWS ID = "aws"`, `Azure ID = "azure"`, `Tencent ID = "tencent"`.
  - `type Provider interface` as in the spec's "Provider contract" section.
  - Shared types moved here from `pkg/manager/results.go`: `Health` (+ add `HealthCLIMissing Health = "CLI MISSING"`), `Finding`, `Warning`, `WarningCode`, `LoginIO`, `LoginEvent`, `LoginResult`, `CreateOutcome`, `ActivateResult{Account string; ApplyHint string; Warnings []Warning}`, `LocalStatus`, `AuthReport{Credentialed map[string]bool; Warnings []Warning}`, `CLIStatus{Bin string; Found bool}`, `Field{Label, Value string}`.
  - `func NewRegistry(providers ...Provider) *Registry`, `func (r *Registry) Get(id ID) (Provider, bool)`, `func (r *Registry) All() []Provider` (stable order: gcp, aws, azure, tencent, then others by ID).

- [ ] **Step 1: Write failing test** `TestRegistryGetAndOrder`: register providers out of order, assert `All()` order and `Get("nope")` returns `ok == false`; `TestRegistryRejectsDuplicateID` (panics, or returns error: pick `NewRegistry` panics on duplicate, assert with `recover`).
- [ ] **Step 2: Run** `go test ./pkg/provider/ -v`. Expected: FAIL (package empty).
- [ ] **Step 3: Implement** types, interface and registry. Add a `fakeProvider` in `fake_test.go` implementing `Provider` with scriptable results; later tasks copy it for dispatch tests.
- [ ] **Step 4: Run** `go test ./pkg/provider/ -v`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(provider): add provider contract and registry`.

### Task 3: Extract GCP behind the contract (no behavior change)

**Files:**
- Create: `pkg/provider/gcp/gcp.go`, `pkg/provider/gcp/gcp_test.go`
- Modify: `pkg/manager/results.go` (replace moved types with aliases, e.g. `type Health = provider.Health`), `pkg/manager/operations.go`, `pkg/manager/status.go`, `pkg/manager/isolate.go`

**Interfaces:**
- Consumes: Task 2 interface; existing `gcloud.Client`, `adc.Store`, `gcloudconfig`.
- Produces: `func gcp.New(opts gcp.Options) *gcp.Provider` where `gcp.Options{GcloudDir, ADCDir, DefaultADC string; Client *gcloud.Client; LookPath func(string)(string,error); Getenv func(string) string}`; `*gcp.Provider` satisfies `provider.Provider`. `manager` type aliases keep `pkg/tui` compiling unchanged.

- [ ] **Step 1: Guard with characterization.** Run `go test ./... -count=1` and record it green on the branch before touching anything. This is the regression oracle; the existing 638-line `manager_test.go` must pass unmodified at the end of this task.
- [ ] **Step 2: Write failing test** `TestGCPLocalMatchesLegacyStatus` in `pkg/provider/gcp/gcp_test.go`: build a temp gcloud dir with one config and a stored ADC, call `Local(acct)`, assert `Status == HealthReady`; and `TestGCPCheckAuthTreatsCredentialedAsAuthed` (fake `gcloud auth list` returns one `ACTIVE`, one `CREDENTIALED`, one other → first two authed). Mirrors fix `11bbb47`.
- [ ] **Step 3: Run** `go test ./pkg/provider/gcp/ -v`. Expected: FAIL.
- [ ] **Step 4: Move the GCP-specific bodies** (`Create`, `Login`, `accountStatus` file logic, `CheckCredentials` parsing, `isolationEnv`, `Delete`) into `gcp.Provider` methods. `Switch`'s config activation moves to `Activate`. Leave `manager` method signatures and `tui.Service` untouched: the manager delegates to the provider for accounts with `ProviderID() == "gcp"`.
- [ ] **Step 5: Run** `go test ./... -count=1`. Expected: PASS, including every pre-existing test unmodified.
- [ ] **Step 6: Commit** `refactor(gcp): move gcp operations behind provider contract`.

### Task 4: Manager dispatch, Overview and credential fan-out

**Files:**
- Modify: `pkg/manager/manager.go` (`Options.Providers []provider.Provider`, registry field), `pkg/manager/status.go`, `pkg/manager/operations.go`, `pkg/manager/isolate.go`
- Test: `pkg/manager/dispatch_test.go`

**Interfaces:**
- Consumes: `provider.Registry`, `fakeProvider` pattern from Task 2.
- Produces: `AccountStatus.Provider provider.ID`, `AccountStatus.TargetLabel string`, `Overview.Providers []ProviderInfo{ID provider.ID; Label string; CLI provider.CLIStatus; Count int}`, `Manager.CheckCredentials(ctx) (CredentialReport, error)` now fans out per provider with `credentialCheckTimeout` each, `Manager.Env(name string) ([]string, error)`, `SwitchResult.ApplyHint string`. `Options.Providers == nil` → default set (`gcp`, later aws/azure/tencent).

- [ ] **Step 1: Write failing tests** in `dispatch_test.go`:
  - `TestOperationsDispatchByProvider`: two fake providers; `Switch("a-aws")` calls only the aws fake.
  - `TestMissingCLIAffectsOnlyThatProvider`: aws fake reports `CLI().Found == false`; `Overview()` returns all accounts, aws rows `Status == HealthCLIMissing`, gcp rows unaffected (Review Focus 3).
  - `TestCheckCredentialsTimeoutIsPerProvider`: aws fake blocks until ctx done, gcp fake answers instantly; with a 50ms timeout the report contains gcp results and an aws `WarnCredentialCheck` warning, total time < 1s (Review Focus 4).
  - `TestSwitchEnvProviderReturnsApplyHint`: env-based fake returns `ApplyHint`; `SwitchResult.ApplyHint` is non-empty and `Warnings` is empty (Review Focus 5).
- [ ] **Step 2: Run** `go test ./pkg/manager/ -run 'Dispatch|MissingCLI|PerProvider|ApplyHint' -v`. Expected: FAIL.
- [ ] **Step 3: Implement dispatch.** `Manager.account(name)` resolves the provider from the registry; unknown provider id → error naming the id (config written by a newer version). `CheckCredentials` runs providers in goroutines with `context.WithTimeout` each and merges `AuthReport`s into `m.credentialed` keyed by account name.
- [ ] **Step 4: Run** `go test ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(manager): dispatch account operations by provider`.

### Task 5: CLI surface

**Files:**
- Modify: `cmd/create.go`, `cmd/list.go`, `cmd/switch.go`, `cmd/info.go`, `pkg/present/text.go`
- Create: `cmd/env.go`
- Test: `cmd/cli_test.go`, `pkg/present/text_test.go` (new if absent)

**Interfaces:**
- Consumes: Task 4 manager API.
- Produces: `gctx create <name> <target> [--provider gcp|aws|azure|tencent]` (default `gcp`); `gctx list [--provider P]` with a PROVIDER column; `gctx env <name>` prints `export KEY='value'` lines (POSIX quoting) for `Manager.Env`; `Manager.Create` rejects a name that exists under any provider with `ErrAccountExists`.

- [ ] **Step 1: Write failing tests** in `cmd/cli_test.go`: `TestCreateDefaultsToGCP` (no flag, same behavior as today), `TestCreateRejectsNameUsedByOtherProvider` (Review Focus 2), `TestListProviderFilter`, `TestEnvPrintsQuotedExports` (value containing a single quote is escaped), `TestEnvGCPPrintsIsolationEnv`.
- [ ] **Step 2: Run** `go test ./cmd/ -run 'Provider|Env|DefaultsToGCP' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** the flag, column and `env` command. Unknown `--provider` value → error listing valid ids. Register `envCmd` in `cmd/root.go`.
- [ ] **Step 4: Run** `go test ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Docs.** Add `--provider` and `env` to `README.md` and `readthedocs/docs`.
- [ ] **Step 6: Commit** `feat(cli): add provider flag, list column and env command`.

### Task 6: AWS provider

**Files:**
- Create: `pkg/provider/aws/aws.go`, `pkg/provider/aws/aws_test.go`, `internal/testutil/fakeaws/main.go`
- Modify: default provider set in `pkg/manager/manager.go`

**Interfaces:**
- Consumes: Task 2 contract.
- Produces: `func aws.New(opts aws.Options) *aws.Provider` with `aws.Options{ConfigFile string; LookPath func(string)(string,error); Getenv func(string) string; Runner provider.Runner}`; `provider.Runner` is a small interface (`Query(ctx, env []string, args ...string) (string, error)`, `Interactive(ctx, io, env, args...) error`) added in this task to `pkg/provider` if Task 3 did not already extract one from `gcloud.Client`. Behavior: Target = profile name; `ValidateTarget` requires `[A-Za-z0-9_.-]{1,64}`; `Local` reads `~/.aws/config` to check `[profile X]` exists (missing → `HealthConfigMissing`); `CheckAuth` runs `aws sts get-caller-identity --profile P --output json` per account and stores the `Arn` as identity; `Login` runs `aws sso login --profile P`; `Env` returns `AWS_PROFILE=P`; `Activate` returns `ApplyHint` `eval "$(<bin> env <name>)"`.

- [ ] **Step 1: Write failing tests**: `TestAWSLocalMissingProfile`, `TestAWSCheckAuthExpired` (fake `sts` exits non-zero with `ExpiredToken` → account not credentialed), `TestAWSCheckAuthParsesArn`, `TestAWSEnv`, `TestAWSLoginUsesProfile` (fake records args), `TestAWSMissingCLI` (`CLI().Found == false` when `LookPath` errors).
- [ ] **Step 2: Run** `go test ./pkg/provider/aws/ -v`. Expected: FAIL.
- [ ] **Step 3: Implement** the provider and fake binary; register in the default set.
- [ ] **Step 4: Run** `go test ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Docs.** AWS section in `README.md` and `readthedocs/docs` (SSO setup prerequisite: profile must exist; gctx does not create AWS profiles).
- [ ] **Step 6: Commit** `feat(aws): add aws provider`.

### Task 7: Azure provider

**Files:**
- Create: `pkg/provider/azure/azure.go`, `pkg/provider/azure/azure_test.go`, `internal/testutil/fakeaz/main.go`
- Modify: default provider set

**Interfaces:**
- Consumes: `provider.Runner` from Task 6.
- Produces: `func azure.New(opts azure.Options) *azure.Provider` with `azure.Options{ConfigRoot string; LookPath ...; Runner provider.Runner}`. Behavior: Target = subscription id (GUID, validated); each account owns `<ConfigRoot>/<name>/` used as `AZURE_CONFIG_DIR`, created with mode 0700; `Login` runs `az login` then `az account set --subscription <id>` in that dir; `CheckAuth` runs `az account get-access-token --output json` with the isolated env and treats a non-zero exit as expired; `Env` returns `AZURE_CONFIG_DIR=<dir>`; `Delete` removes the isolated dir (only if it is inside `ConfigRoot`).

- [ ] **Step 1: Write failing tests**: `TestAzureValidateTargetRejectsNonGUID`, `TestAzureCreateMakesPrivateDir` (mode `0700`), `TestAzureEnvIsolatesPerAccount` (two accounts → two different dirs), `TestAzureCheckAuthExpired`, `TestAzureDeleteRefusesPathOutsideRoot` (tampered `Settings["dir"]` → error, nothing removed), `TestAzureLoginSetsSubscription` (fake records both calls in order).
- [ ] **Step 2: Run** `go test ./pkg/provider/azure/ -v`. Expected: FAIL.
- [ ] **Step 3: Implement** provider and fake binary; register.
- [ ] **Step 4: Run** `go test ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Docs.** Azure section.
- [ ] **Step 6: Commit** `feat(azure): add azure provider`.

### Task 8: Tencent provider (spike first)

**Files:**
- Modify: `docs/superpowers/specs/2026-10-21-multi-cloud-design.md` (record spike findings, close D8)
- Create: `pkg/provider/tencent/tencent.go`, `pkg/provider/tencent/tencent_test.go`, `internal/testutil/faketccli/main.go`

**Interfaces:**
- Consumes: `provider.Runner`.
- Produces: `func tencent.New(opts tencent.Options) *tencent.Provider`; exact commands and env are set by the spike result. The contract methods and test set mirror Task 6 (profile target, `CheckAuth`, `Login`, `Env`, `Local`, missing CLI).

- [ ] **Step 1: Spike (no code merged).** With a real `tccli` and a real Tencent account, document in the spec: how profiles are stored, how to select one (flag or env), how interactive login works, and which cheap call distinguishes expired from valid credentials. If profile isolation is not possible, stop and bring the alternative to the user; do not guess.
- [ ] **Step 2: Write failing tests** per the spike: `TestTencentLocalMissingProfile`, `TestTencentCheckAuthExpired`, `TestTencentEnv`, `TestTencentLogin`, `TestTencentMissingCLI`.
- [ ] **Step 3: Run** `go test ./pkg/provider/tencent/ -v`. Expected: FAIL.
- [ ] **Step 4: Implement** provider and fake binary; register.
- [ ] **Step 5: Run** `go test ./... -count=1`. Expected: PASS.
- [ ] **Step 6: Docs.** Tencent section.
- [ ] **Step 7: Commit** `feat(tencent): add tencent provider`.

### Task 9: TUI multi-cloud

> **Superseded:** do not implement this task. It is replaced by Tasks 3 to 10 of `docs/superpowers/plans/2026-10-22-tui-v2-ai.md`. Run that plan after Task 7 here and before Task 10.

**Files:**
- Modify: `pkg/tui/service.go` (add `Env(name string) ([]string, error)`, `Providers() []manager.ProviderInfo`), `pkg/tui/keys.go`, `pkg/tui/main_keys.go`, `pkg/tui/model.go`, `pkg/tui/view.go`, `pkg/tui/modals.go`, `pkg/tui/styles.go`, `pkg/tui/fakeservice_test.go`
- Test: `pkg/tui/view_layout_test.go`, `pkg/tui/keys_bindings_test.go`, `pkg/tui/modals_switch_test.go`, new `pkg/tui/multicloud_test.go`

**Interfaces:**
- Consumes: Task 4 `Overview.Providers`, `AccountStatus.Provider`/`TargetLabel`, `SwitchResult.ApplyHint`.
- Produces: `columnSet.cloud int` + `showCloud bool`; `type cloudFilter int` (0 = All, then one per registered provider in registry order); `Model.cloud cloudFilter`; keys `NextCloud` (`tab`), `PrevCloud` (`shift+tab`), `JumpCloud` (`1`–`5`), `Env` (`e`); `createForm.provider provider.ID` as the first field, cycled by `←`/`→`; `func cloudBadge(id provider.ID, st styles) string` returning the 3-letter badge. Layout and copy follow the spec's "TUI design" section exactly.

- [ ] **Step 1: Write failing tests** in `multicloud_test.go`: `TestCloudTabFiltersRows` (tab to AWS → only aws rows visible, counts in the tab row match), `TestJumpKeySelectsCloud` (`2` → GCP), `TestRowShowsCloudBadgeWide` (≥120 cols, badge text `AWS` in row), `TestNarrowFoldsBadgeIntoName` (<90 cols, no CLOUD column header, row starts with `AWS `), `TestTargetHeaderFollowsCloudTab` (AWS tab → `PROFILE`), `TestCreateFormCloudPickerChangesLabel` (`→` flips label PROJECT→PROFILE and validation), `TestCreateDisabledWhenCLIMissing`, `TestSwitchEnvProviderShowsApplyHint` (status line contains the hint, no "switched" claim), `TestHintsIncludeCloudAndEnv`, `TestHelpListsCloudKeys`, `TestMissingCLITabDimmed`. Extend `fakeservice_test.go` with a multi-provider overview fixture.
- [ ] **Step 2: Run** `go test ./pkg/tui/ -run 'Cloud|Badge|Narrow|Target|Hints|Help' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** keys, model state, filtering (combine with the existing text filter: cloud filter first, then `/` query), column layout, header (`env:` hint when the active account is not GCP), tab row, create-form picker, details rows from `Provider.Details`, and the apply-hint status. Keep existing GCP-only snapshots passing: with only GCP accounts the CLOUD column and tab row still render (single tab) but all current assertions on rows must be updated deliberately, not weakened.
- [ ] **Step 4: Run** `go test ./pkg/tui/ -count=1` and `go test ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Manual check** with `go build -o gctx main.go && ./gctx tui` at widths 160, 120, 90, 70 (resize the terminal); confirm badges, tabs, create-form picker, and the narrow fold.
- [ ] **Step 6: Docs.** New screenshots/keys in `README.md` and `readthedocs/docs`; update the hint line text in docs.
- [ ] **Step 7: Commit** `feat(tui): multi-cloud accounts, cloud tabs and provider-aware create`.

### Task 10: Rename and final docs sweep

**Gate:** spec decision D7 answered. Examples below use `cctx`; substitute the chosen name.

**Files:**
- Modify: `go.mod` (module path), every import of `github.com/k0wl0n/gctx`, `main.go`, `cmd/root.go` (`Use`), `Taskfile.yml`, `.goreleaser*` and Homebrew/Scoop config, `mise.toml`, `README.md`, `readthedocs/`, `CONTRIBUTING.md`
- Create: `pkg/config/migrate.go`, `pkg/config/migrate_test.go`
- Test: `cmd/cli_test.go`

**Interfaces:**
- Produces: `func MigrateLegacyDir(home string) (migrated bool, err error)` in `pkg/config`: if `~/.config/<new>` does not exist and `~/.config/gctx` does, copy it (config + `adc/`, preserving file modes) and leave the old directory in place; never overwrites an existing new directory. `GetConfigDir()` returns the new path. Env vars `GCTX_ACCOUNT`, `GCTX_DEBUG`, `GCTX_NO_TUI` remain valid; the new `CCTX_*` names are read first, then the `GCTX_*` fallback.

- [ ] **Step 1: Write failing tests**: `TestMigrateCopiesLegacyDir` (config and an ADC file with mode `0600` arrive with the same mode), `TestMigrateNeverOverwritesNewDir`, `TestMigrateNoopWithoutLegacy`, `TestLegacyEnvVarFallback` (`GCTX_NO_TUI=1` still disables the TUI), `TestGctxAliasStillRuns` (built alias binary behaves like the new one).
- [ ] **Step 2: Run** `go test ./pkg/config/ ./cmd/ -run 'Migrate|LegacyEnv|Alias' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** migration (called once from `manager.New`) and the env-var fallback.
- [ ] **Step 4: Mechanical rename** in one commit-sized pass: module path (`go mod edit -module ...` then update imports), binary name, `Use`, goreleaser/Homebrew/Scoop, docs. Ship a `gctx` alias (second goreleaser build or a symlink in the formula) for one release.
- [ ] **Step 5: Run** `go build ./... && go test ./... -count=1`. Expected: PASS, and `grep -rn 'k0wl0n/gctx' --include='*.go' .` returns nothing if the module path changed.
- [ ] **Step 6: Final docs sweep.** README overview, install instructions, command reference, TUI screenshots, `readthedocs/mkdocs.yml` nav; remove any remaining GCP-only wording.
- [ ] **Step 7: Commit** `feat!: rename gctx to cctx and migrate legacy config dir`.
