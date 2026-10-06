# Agent B3: subprocess flows (login, run, shell), tokenizer, debug log

Agent id: `b3-exec`. Read `agents/shared-context.md` and `agents/reports/stage-a.md` (the contract) first.
Work only inside the worktree path you were given, on its branch.

## You own (edit only these)
- `pkg/tui/exec.go`, `pkg/tui/tokenize.go`, `pkg/tui/debuglog.go` (new)
- new test files `pkg/tui/exec_*_test.go`, `pkg/tui/tokenize_test.go`, `pkg/tui/debuglog_test.go`

## Tasks
- 6.1: login.
  - `loginExec` implements `tea.ExecCommand`: banner; `svc.Login` with `pkg/present` progress output; on failure, print the error and `Press Enter to return to gctx` and wait for Enter.
  - `beginLogin` runs it through `m.exec`.
  - `handleExecDone`: reload; clear the session `AUTH REQUIRED`; re-run the credential check; show the spec status messages (complete / cancelled / ADC not saved).
- 7.1: tokenizer, exactly per specs/credential-safety. Includes the metacharacter rejection and the Windows `.cmd`/`.bat` list.
- 7.2: run prompt, as a type implementing the contract's `modal` interface, returned by `newRunPrompt`. It covers:
  - the ADC-missing hint
  - in-memory history (50 entries, ↑/↓)
  - empty input not submitted; inline tokenizer errors
  - `runExec`: banner, run, exit line, Enter wait
  - exit code mapped to a message
- 7.3: shell. `shellExec`: banner; resume immediately; exit message with status; works without gcloud.
- 8.1: debug logging in `debuglog.go`.
  - Enabled by `GCTX_DEBUG=1` or `Options.Debug`.
  - Writes to `os.UserCacheDir()/gctx/tui-debug.log` with mode 0600; truncate when over 1 MiB.
  - Logs only at the op boundary; records only the verb for run.
  - Expose `newDebugLogger(enabled bool) (*slog.Logger, io.Closer, error)` for `run.go`, and request the wiring.

## Tests
- `tokenize_test.go`: a table covering the three credential-safety scenarios plus edge cases.
- Automated parts of E2E-04, E2E-04b, E2E-05, E2E-06 and E2E-11 (b, c, d at model level), using `execSync` with a real `manager.Manager` + `testutil.NewEnv`. Use `SHELL=env.FakeShell()` for the shell.
- A debug log test proving `db-pass` isn't logged when running `secrets versions access latest --secret=db-pass`.

Commit per task. Tick your tasks in tasks.md. Write `agents/reports/b3-exec.md` and commit it. List every wiring request, such as `run.go` debug logger setup and any `Update` changes.
