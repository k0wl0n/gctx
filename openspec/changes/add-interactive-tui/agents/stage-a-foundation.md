# Agent A: foundation (sequential; must finish before agents B1–B4)

Agent id: `stage-a`. Read `agents/shared-context.md` first.
Work in the main checkout on branch `feat/interactive-tui`.

## Part 1: manager refactor
Do tasks 2.1, 3.1–3.9, then 2.6:
- 2.1, 3.1–3.9: `manager.Paths`, `NewWithOptions`, structured operations, `pkg/present`. Each `cmd/*.go` becomes thin. Make the 4 allowed output changes in separate commits.
- 2.6: delete `pkg/watcher`, `pkg/adc/legacy.go` and `pkg/gcloud/legacy.go`.
- Run the full `go test ./...` (including the slow create/login tests) after 3.7, and `go test -race ./...` at 3.9.
- Add domain tests per test-plan.md layer 2.

## Part 2: TUI contract for the parallel agents
Do tasks 4.1 (dependencies), 4.2 (`cmd/tty.go`, `cmd/tui.go`, `const tuiDefaultEnabled = false`, root-help golden gains the `tui` line), and 4.3 (skeleton).

The skeleton is the contract that lets four agents work in parallel without touching the same file. Create these files so they compile, with the exact split below:

| File | Owner after you | Contents you create |
|---|---|---|
| `pkg/tui/service.go` | integration | `type Service interface` with every manager method the TUI needs (Overview, CheckCredentials, Info, Switch, Create, Login, SaveCredentials, Delete, RunCommand, ShellCommand) plus access to validation (`manager.ValidateAccountName`, `manager.ValidateProjectID`). |
| `pkg/tui/model.go` | integration | see "model.go" below |
| `pkg/tui/run.go` | integration | `Run(ctx, svc Service, opts Options{Version string; Debug bool}) error`: program setup, error mapping, terminal restore. |
| `pkg/tui/keys.go` | B1 | one `key.Binding` table covering every binding in specs/interactive-tui "Keybindings" (Navigation / Actions / General groups) |
| `pkg/tui/styles.go` | B1 | Lip Gloss styles, glyph sets (Unicode and `GCTX_ASCII`) |
| `pkg/tui/main_keys.go` | B1 | stubs: `updateMain`, `updateFilter`, `updateOverlay`, `handleCreds` |
| `pkg/tui/view.go` | B1 | stubs: `viewMain`, `viewOverlay` |
| `pkg/tui/modals.go` | B2 | stubs: `updateModal`, `viewModal`, `handleOpDone`; and the action entry points B1 calls: `startSwitch(name string) (Model, tea.Cmd)`, `openSave`, `openDelete`, `openCreate`, `openRun`, `startShell`, `startLogin` (the last three just forward to B3's functions, see below) |
| `pkg/tui/exec.go` | B3 | stubs: `handleExecDone`, `beginLogin(name string) (Model, tea.Cmd)`, `beginShell(name string) (Model, tea.Cmd)`, `newRunPrompt(name string) modal`, the `loginExec`/`runExec`/`shellExec` types |
| `pkg/tui/tokenize.go` | B3 | stub `func tokenize(input, gcloudBin, goos string) ([]string, error)` |
| `pkg/tui/testing_test.go` | integration | helpers `newTestModel`, `press`, `resize`, `drain`, `screen`, `execSync` |
| `pkg/tui/fakeservice_test.go` | integration | an in-memory fake `Service` with scripted results, recorded calls, and an optional blocking gate |

### model.go
- The `Model` struct with every field from design.md "Model and state", including `exec func(tea.ExecCommand, tea.ExecCallback) tea.Cmd` and `now func() time.Time`.
- `type modal interface { update(Model, tea.Msg) (Model, tea.Cmd); view(Model) string }`, so B2 and B3 can each add modal types in their own files.
- All message types: `credsCheckedMsg`, `opDoneMsg`, `execDoneMsg`, `statusExpiredMsg`.
- Helpers: `operation`, `statusMsg`, `setStatus(text, level)`, `selectedAccount()`, `reload()`, `busy()`.
- `Init`.
- `Update` only dispatches, in this order:
  1. `tea.WindowSizeMsg` → store size
  2. async messages → `handleOpDone` / `handleExecDone` / `handleCreds`
  3. the too-small gate (only `q`/`ctrl+c` act)
  4. `m.modal != nil` → `updateModal`
  5. `m.filter.editing` → `updateFilter`
  6. help/details screen → `updateOverlay`
  7. otherwise `updateMain`
- `View` builds a `tea.View{AltScreen: true, ReportFocus: true}` around `viewMain`/`viewOverlay`/`viewModal`.

Every stub must compile and have one trivial test, so the parallel agents start from green.

## Part 3: hand-off
1. Commit the skeleton as `feat: add gctx tui command (experimental)`.
2. Tick the tasks you completed in tasks.md and commit.
3. Write your report (per shared-context.md) with the final commit SHA. Include the exact exported/unexported signatures of the contract functions above, so the parallel agents and the integration agent can rely on them.
