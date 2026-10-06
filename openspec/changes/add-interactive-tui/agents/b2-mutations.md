# Agent B2: mutating actions and dialogs

Agent id: `b2-mutations`. Read `agents/shared-context.md` and `agents/reports/stage-a.md` (the contract) first.
Work only inside the worktree path you were given, on its branch.

## You own (edit only these)
- `pkg/tui/modals.go`
- new test files `pkg/tui/modals_*_test.go` and `pkg/tui/ops_*_test.go`

## Tasks
- 5.1: switch as a background operation: spinner, warnings mapped to messages, re-apply on the active account. Implement it in `startSwitch` and `handleOpDone`.
- 5.2: operation concurrency: single in-flight op, the `Busy:` message, `q` while busy, double `ctrl+c` within 3 s with a fake clock, stale-seq drop. Put helpers in modals.go; if `Update` in model.go needs a change, request it.
- 5.3: save-ADC dialog.
- 5.4: delete dialog. Initial focus `[Cancel]`; the checkbox is disabled when the config is gcloud's active one; select the nearest row afterwards.
- 6.2: create form.
  - fields, all validation messages, the reused-config second Enter
  - background `Create`, then chain to login by calling `startLogin(name)`; it's a B3 stub on your branch, so test that it is called
  - select the new account; show the outcome message, including when authentication did not complete

`updateModal` must delegate to `m.modal.update(...)` through the `modal` interface, so B3's run prompt works after merge.

## Tests
- every scenario in specs/interactive-tui for switch, operation concurrency, create flow, delete flow, save ADC flow
- credential-safety "Cancel save"
- automated parts of E2E-02, E2E-03, E2E-07 (the form and Create part) and E2E-08, using a real `manager.Manager` + `testutil.NewEnv` where the test plan says "A"

Commit per task. Tick your tasks in tasks.md. Write `agents/reports/b2-mutations.md` and commit it.
