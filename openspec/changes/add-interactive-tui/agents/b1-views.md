# Agent B1: views, navigation, filter, health display

Agent id: `b1-views`. Read `agents/shared-context.md` and `agents/reports/stage-a.md` (the contract) first.
Work only inside the worktree path you were given, on its branch.

## You own (edit only these)
- `pkg/tui/main_keys.go`, `pkg/tui/view.go`, `pkg/tui/styles.go`, `pkg/tui/keys.go`
- `pkg/manager/status_bench_test.go`
- new test files `pkg/tui/view_*_test.go`, `pkg/tui/keys_*_test.go`, `pkg/tui/security_test.go`

## Tasks
- 4.4–4.11:
  - table, header, responsive layout, navigation
  - filter, details view, help screen
  - credential check, reload, empty state, error state, status message expiry
  - performance guard, security assertions
- 7.4: gcloud-unavailable gating inside `updateMain`.

In `updateMain`, call the action entry points from the contract (`startSwitch`, `openSave`, `openDelete`, `openCreate`, `openRun`, `startShell`, `startLogin`) for `enter`, `a`, `ctrl+d`, `c`, `r`, `s`, `l`. Those functions are stubs on your branch; that's fine. Test only that the right entry point is reached (for example, via the fake service's recorded calls, or a status message). Behavior is tested by B2 and B3.

## Tests (test-plan.md layer 3)
- keybindings, selection, filter, layout breakpoints (widths 59/60/89/90/119/120, heights 11/12)
- three stripped breakpoint goldens
- header states, empty and error states, status message timing (fake clock)
- credential check responsiveness and stale-seq drop
- the security test (`SECRET-REFRESH-` never rendered; no clipboard package in `go list -deps ./pkg/tui`)
- automated parts of E2E-01, E2E-09 and E2E-10 (model level)

Commit per task. Tick your tasks in tasks.md. Write `agents/reports/b1-views.md` and commit it.
