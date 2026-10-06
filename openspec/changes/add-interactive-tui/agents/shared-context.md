# Shared context: gctx interactive TUI

Every agent reads this file first. Work autonomously: don't ask questions, make reasonable decisions, and record deviations in commit messages and in your report. Never push, never open a PR.

## Source of truth
All under `openspec/changes/add-interactive-tui/`:
- `tasks.md`: the checklist. Tick `- [x]` only for tasks you completed and verified.
- `design.md`: architecture, decisions D1–D13, domain API sketch, TUI model and state, testing seams, pinned versions.
- `specs/*/spec.md`: the required behavior (account-management, cli-commands, interactive-tui, credential-safety).
- `test-plan.md`: test layers, E2E-01..12, acceptance criteria.

## Already done (branch `feat/interactive-tui`)
- Group 1:
  - fake gcloud in `internal/testutil/fakegcloud`
  - harness in `internal/testutil/harness.go` (`NewEnv`, `SeedAccount`, `Invocations`, `InvocationLines`, `SetActive`, `SetGcloudActiveConfig`, `GcloudProperty`, `HideGcloud`, `FakeShell`…)
  - CLI tests that freeze current output in `cmd/cli_test.go`, with goldens in `cmd/testdata`
  - CI matrix with SHA-pinned actions; dependabot
- Group 2 (except 2.1 and 2.6):
  - `pkg/config` (`LoadFrom`, atomic `Save`, sorted `ListAccounts`)
  - `pkg/adc/store.go` (`Store`)
  - `pkg/gcloud/client.go` (`Client`, `StepError` keeping the old error text)
  - `pkg/gcloudconfig`
- The old code still lives in `pkg/adc/legacy.go`, `pkg/gcloud/legacy.go` and `pkg/watcher`. The current `pkg/manager/manager.go` still uses it.

## Rules
- Before every commit: `go build ./... && go vet ./... && go test -short ./...`. Keep `go mod tidy -diff` empty.
- Conventional Commits, lowercase imperative subjects (`refactor:`, `fix:`, `test:`, `feat:`, `docs:`).
- The CLI tests in `cmd/cli_test.go` must stay green. The only output changes allowed are the 4 listed in `specs/cli-commands` "Stable CLI output". Make each in its own commit, regenerating goldens with `go test ./cmd -run <Test> -update`.
- Dependencies: exact versions, each ≥ 7 days old (check with `curl -s https://proxy.golang.org/<module>/@v/<ver>.info`). Planned versions:
  - charm.land/bubbletea/v2 v2.0.10
  - charm.land/bubbles/v2 v2.2.1
  - charm.land/lipgloss/v2 v2.0.6
  - golang.org/x/term v0.46.0
  - github.com/creack/pty v1.1.24 (test-only, `//go:build !windows`)
- Match existing style; minimal comments. One interface only (`tui.Service`).
- Never print, log or render ADC contents or tokens. Seeded ADC files contain `SECRET-REFRESH-<name>`; tests assert it never appears in output or logs.
- Use absolute paths. If you were given a worktree path, run every command inside it (`cd <path> && …` or `git -C <path>`), never in the main checkout.
- Edit only the files you own (listed in your agent file). Anything you need changed elsewhere goes in your report under "Requests for the integration agent".

## Gotchas
- Fake gcloud failure/latency injection: `FAKE_GCLOUD_FAIL="verb:exit[:stderr];…"` and `FAKE_GCLOUD_SLEEP="verb:ms"`. Verbs: `configurations-create`, `configurations-activate`, `configurations-delete`, `configurations-list`, `config-set`, `config-get-value`, `auth-login`, `auth-adc-login`, `auth-list`, `other`. `FAKE_GCLOUD_ADC_UNCHANGED=1` makes ADC login leave the file untouched. `FAKE_GCLOUD_ACCOUNT` sets the login email.
- The fake `configurations create` also activates the new config (like real gcloud). Deleting the active config fails.
- The harness sets `CLOUDSDK_CONFIG` to the directory the old code derived from HOME/APPDATA, so tests are valid before and after the refactor.
- Preserve existing error strings and stdout text exactly, for example `auth login failed: %w`, `ADC auth failed: %w`, `account '%s' not found`, and the switch warning texts.
- Account-scoped gcloud calls pass `CLOUDSDK_ACTIVE_CONFIG_NAME=<config>` explicitly. Read the email with `config get-value account` under that env.
- Bubble Tea v2:
  - `View()` returns `tea.View` (set `AltScreen`, `ReportFocus`).
  - `msg.String()` yields `"G"`, `"R"`, `"ctrl+d"`, `"space"`.
  - Use `tea.Exec` with our own `tea.ExecCommand`, always through the `Model.exec` seam so tests can run it synchronously.
- Bare-`gctx` routing lives in `cmd.Execute()` before Cobra (root stays non-runnable), gated by `const tuiDefaultEnabled` (false until task 9.1).

## Report format
When done, write your report to `openspec/changes/add-interactive-tui/agents/reports/<your-agent-id>.md`, commit it, and also return it as your final message. Include:
- branch and final commit SHA
- tasks completed
- tasks not completed, with reasons
- deviations from spec or design
- requests for the integration agent (exact wiring or edits needed in files you don't own)
- test command output summary
