# Agent C: integration and release gate (after B1–B4 finish)

Agent id: `stage-c`. Read `agents/shared-context.md` and all of `agents/reports/*.md` (on the B branches) first.
Work in the main checkout on branch `feat/interactive-tui`. You may edit any file.

## Steps
1. Merge the B branches into `feat/interactive-tui` in this order: B1 (views), B2 (mutations), B3 (exec), B4 (docs). The orchestrator gives you the branch names.
   - Resolve conflicts. tasks.md conflicts are expected; keep every `[x]`.
   - After each merge, run `go build ./... && go vet ./... && go test ./...`.
2. Apply every "Requests for the integration agent" item from the four reports. Typical items:
   - wire `updateMain` keys → `startSwitch`/`openSave`/`openDelete`/`openCreate`/`openRun`/`startShell`/`startLogin` → B3's `beginLogin`/`beginShell`/`newRunPrompt`
   - wire the debug logger into `run.go`
   - any `Update` dispatch change
   Then run `go test -race ./...`.
3. Run the integration tests from test-plan.md layer 4 end to end, adding any E2E automated parts no agent covered: the full chain from E2E-07 create to login, and E2E-11e double `ctrl+c`.
4. Tasks:
   - 8.2: PTY smoke tests 6.1–6.4 in `cmd/pty_unix_test.go`, using `github.com/creack/pty` v1.1.24 test-only.
   - 8.3: `grep` that `pkg/tui` has no `os.Exit` or `log.Fatal`; add a panic-restore test.
   - 9.1: set `tuiDefaultEnabled = true` and remove "experimental" from `tui`'s short help. Commit as `feat: open interactive TUI when gctx runs without arguments`, with a body explaining the bare-`gctx` change and `GCTX_NO_TUI`.
   - 9.2: acceptance gate. Write `openspec/changes/add-interactive-tui/traceability.md`, mapping every requirement in specs/*.md to a test name or a manual-matrix row number. Every requirement must be covered.
   - Regenerate the CLI reference with `go run main.go docs && cp README.md readthedocs/docs/index.md`.
   - 10.4: run the goreleaser snapshot only if `goreleaser` is installed; record the binary size delta versus the `main` branch build.
5. Remove the worktrees and branches the orchestrator created (`git worktree remove <path>`, `git branch -d <branch>`).
6. Run `openspec validate add-interactive-tui --strict`. Don't archive (10.5 happens after release).

## Leave unchecked, and list in the report with reasons
- 1.5 "CI green" (needs a push)
- 8.4 / E2E-12 and the manual matrix (need a person on Windows, macOS, Linux)
- mkdocs build or goreleaser, if not installed
- 10.5 (after release)

Write `agents/reports/stage-c.md`, commit it, and return it.
