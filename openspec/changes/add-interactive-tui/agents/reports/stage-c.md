# Stage C — Integration Report

**Branch:** `feat/interactive-tui`  
**Agent:** Stage C (integration)  
**Date:** 2026-10-13

---

## Summary

All four feature branches were merged into `feat/interactive-tui` in the required order
(tui/views → tui/mutations → tui/exec → tui/docs). All B2 and B3 wiring requests were
applied, all tasks from the checklist were completed, and `go test ./...` passed clean
after every step.

---

## Merge log

| Step | Branch merged | Conflicts | Result |
|---|---|---|---|
| 1 | tui/views | none | all tests pass |
| 2 | tui/mutations | none | all tests pass |
| 3 | tui/exec | none | all tests pass |
| 4 | tui/docs | none | all tests pass |

`tasks.md` auto-merged correctly across all four merges; no manual conflict resolution was required.

---

## Wiring applied (B2 + B3 requests)

### pkg/tui/model.go
- Added `debugLog *slog.Logger` and `goos string` fields to `Model`.
- Split `New()` into `New()` + `NewWithGOOS()` (pure factory, no side effects).
- Default `debugLog` is a no-op handler writing to `io.Discard` (safe if nil is never checked).

### pkg/tui/run.go
- Reads `GCTX_DEBUG` env var in addition to the `--debug` flag.
- Calls `newDebugLogger` to open a mode-0600 log file when debug is enabled.
- Passes `runtime.GOOS` to `NewWithGOOS` so the tokenizer uses the real platform.
- Assigns `model.debugLog` before handing the model to `tea.NewProgram`.

### pkg/tui/exec.go
- Added `ctx context.Context` field to `loginExec` struct.
- `loginExec.Run()` uses the stored context instead of `nil`.
- `beginLogin` passes `context.Background()` when called from the TUI.
- GOOS injection: `runPrompt.update` reads `m.goos`; falls back to `"linux"` only if empty.
- Debug logging added to `handleRunDone` (account, exit-code level).

### pkg/tui/modals.go
- `handleOpDone` / `opCreate` case: after a successful create, immediately calls
  `startLogin(res.Account)` so the create→login chain fires automatically.
- `startSwitch` emits a `m.debugLog.Info("op", ...)` entry at operation start.

---

## Tasks completed

### Task 8.2 — PTY smoke tests (`cmd/pty_unix_test.go`)
Five tests under build tag `//go:build !windows`:
- `TestPTY_NonTTY_PrintsNonTTYMessage`
- `TestPTY_LaunchesWithoutCrash`
- `TestPTY_GCTX_NO_TUI_SuppressesTUI`
- `TestPTY_GCTX_DEBUG_Accepted`
- `TestPTY_ListCommand`

Uses `github.com/creack/pty v1.1.24` (added to go.mod/go.sum).  
Binary is built to `/tmp/gctx-pty-<TestName>` to avoid macOS noexec on `/var/folders`.

### Task 8.3 — Security invariants (`pkg/tui/security_test.go`)
- `TestNoOsExitOrLogFatalInTUI`: greps non-test `.go` files in `pkg/tui` for
  `os.Exit|log.Fatal`; fails if any match is found.
- `TestPanicRestoreDoesNotKillProcess`: calls `mapRunError` with a simulated panic
  error and asserts a non-nil, non-re-panicking result.
- `TestNoClipboardPackageInDeps`: uses module path `github.com/k0wl0n/gctx/pkg/tui`
  (no hardcoded worktree path).

### Task 9.1 — TUI on by default
- `cmd/root.go`: `tuiDefaultEnabled = true`.
- `cmd/tui.go`: `Short` changed from `"Interactive terminal UI (experimental)"` to
  `"Interactive terminal UI"`.
- `cmd/testdata/root-help.golden`: golden file updated to match.

### Task 9.2 — Traceability matrix
`openspec/changes/add-interactive-tui/traceability.md` — covers all requirements in:
- `specs/interactive-tui/spec.md` (all sections)
- `specs/credential-safety/spec.md`
- `specs/cli-commands/spec.md`
- `specs/account-management/spec.md` (delegated to existing manager tests)

Five scenarios remain deferred (Windows PTY, manual terminal-restore, real gcloud
integration, ADC file permissions, NO_COLOR visual check) — listed in the
"Uncovered requirements" section of the matrix.

### CLI reference regeneration
- `go run . docs` regenerated all docs under `readthedocs/docs/` including the new
  `gctx_tui.md` page.
- `cp README.md readthedocs/docs/index.md` updated the readthedocs entry point.

---

## Worktree and branch cleanup

All four worktrees removed and their branches deleted:

```
git worktree remove gctx-wt-views
git worktree remove gctx-wt-mutations
git worktree remove gctx-wt-exec
git worktree remove gctx-wt-docs
git branch -d tui/views tui/mutations tui/exec tui/docs
```

---

## openspec validate

```
openspec validate add-interactive-tui --strict
Change 'add-interactive-tui' is valid
```

---

## Commits in this session

| SHA | Message |
|---|---|
| `d282bbb` | feat: open interactive TUI when gctx runs without arguments |
| `7f82687` | feat: wire debug logger, GOOS injection, login context, create→login chain |
| `e1fd25d` | docs: add traceability matrix and regenerate CLI reference (9.2) |

---

## Deferred items (not Stage C scope)

| Item | Reason |
|---|---|
| 1.5 CI green | Requires pushing to remote and waiting for CI |
| 8.4 / E2E-12 Windows PTY | Needs a Windows machine |
| M-04 terminal restore | Needs a human to verify cursor/echo state |
| M-05 isolated run | Needs real gcloud installed |
| M-06 ADC file permissions | Needs a real login flow |
| 10.5 archive | After release only |

---

## Test suite status

`go test ./...` passed clean on `feat/interactive-tui` at time of handoff.  
All packages: `pkg/tui`, `pkg/manager`, `cmd`, and supporting packages — green.
