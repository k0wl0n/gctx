Phases map to task groups as in design.md D13. Each group ends in a releasable `main`. Every task lists the files it touches and how to verify it. References: specs in `specs/`, infrastructure and E2E IDs in `test-plan.md`.

## 1. Baseline and safety net (Phase 0 — no behavior change)

- [ ] 1.1 Tidy modules and raise the toolchain.
  - Files: `go.mod`, `go.sum`, `mise.toml`.
  - Run `go mod tidy` (promotes `go-fuzzyfinder` to a direct dependency). Set `go 1.26.0` and `toolchain go1.26.8`, or the newest 1.26.x that is ≥ 7 days old (D9). Set `mise.toml` `go` to the same patch, and pin `task`, `gcloud`, and `goreleaser` to exact versions instead of `latest`.
  - Verify: `go mod tidy -diff` empty; `go build ./...` and `go vet ./...` pass.
- [ ] 1.2 Add the fake gcloud binary.
  - Files: `internal/testutil/fakegcloud/main.go`.
  - Implement the verbs, JSONL invocation log, failure/latency injection, and `FAKE_GCLOUD_ADC_UNCHANGED` exactly as described in test-plan.md "Test infrastructure". Add a "shell" mode that prints its environment and exits with `$FAKE_SHELL_EXIT`.
  - Verify: `internal/testutil/fakegcloud/main_test.go` builds it and asserts each verb's file effects and log record on all 3 OSes.
- [ ] 1.3 Add the test harness.
  - Files: `internal/testutil/harness.go`.
  - `NewEnv(t)` sets `HOME`, `USERPROFILE`, `APPDATA`, `CLOUDSDK_CONFIG` (equal to the gcloud dir the current code derives — see design "Testing seams"), and `PATH`. Provide the seed and inspection helpers.
  - Verify: harness self-test seeds an account and reads it back via the current `config.Load()`.
- [ ] 1.4 Write CLI characterization tests against the **unmodified** code.
  - Files: `cmd/cli_test.go`, `cmd/testdata/*.golden`.
  - Cover every row of test-plan.md layer 1. Tag create/login cases with `testing.Short()` skip (they incur the existing 5 s wait).
  - Verify: `go test ./cmd` passes locally with no production code changes in the same commit.
- [ ] 1.5 CI matrix and supply-chain hygiene.
  - Files: `.github/workflows/ci.yml`, `.github/workflows/auto-release.yml`, `.github/workflows/release-patch.yml`, `.github/dependabot.yml`.
  - Make the `test` job a matrix over `ubuntu-latest`, `macos-latest`, `windows-latest`. Run `go vet ./...`, `go mod tidy -diff`, `go mod verify`, and `go test -short ./...` on every OS, plus a full `go test ./...` (slow cases included) on ubuntu. Pin every action to a full commit SHA, with a version comment. Add `dependabot.yml` for `gomod` and `github-actions` with `cooldown: { default-days: 7 }`.
  - Verify: CI is green on all three OSes for the PR.
- [ ] 1.6 Fix stale developer material.
  - Files: `Taskfile.yml` (`demo:create` drops `--auto-save`), `.planning/STATE.md` and `.planning/ROADMAP.md` (mark 1.2 complete; add a pointer to `openspec/changes/add-interactive-tui`).
  - Verify: `task demo:create --dry` shows no `--auto-save`.

## 2. Domain foundations (Phase 1)

- [ ] 2.1 Introduce `manager.Paths` and the default resolver.
  - Files: `pkg/manager/manager.go`.
  - Resolve `GctxDir`, `ADCDir`, `GcloudDir` (honoring `CLOUDSDK_CONFIG`; `%APPDATA%\gcloud` on Windows), and `DefaultADC` from injected `getenv`/`goos`/home.
  - Verify: table test covers Linux, macOS, and Windows inputs with and without `CLOUDSDK_CONFIG`.
- [ ] 2.2 Make config persistence atomic and injectable.
  - Files: `pkg/config/config.go`.
  - Add `LoadFrom(path)` and `SaveTo(path)`, keeping `Load()`/`Save()` as wrappers. Saves use temp file + `Sync` + `Rename`, with mode `0644` and directory `0755`. `ListAccounts` returns accounts sorted by name.
  - Verify: unit tests for round-trip schema equality against a fixture, no leftover temp file, previous content intact on forced rename failure, and sorted order.
- [ ] 2.3 Make ADC storage injectable and add bounded removal.
  - Files: `pkg/adc/adc.go`.
  - Add `adc.Store{Dir, DefaultPath}` with `Save`, `Restore` (tmp + rename, `0600`), `Validate` (`json.Valid`, error messages without content), `Snapshot`/`Changed` (replaces `pkg/watcher`), `Equal(a, b)`, and `Remove(name, recordedPath)`, which only deletes inside `Dir`. Remove the unused `GetADCEmail` content read.
  - Verify: unit tests for permissions (POSIX), outside-store refusal, and a corrupt-file message that excludes file bytes.
- [ ] 2.4 Turn gcloud calls into a `Client`.
  - Files: `pkg/gcloud/gcloud.go`.
  - `Client{Bin, Env}` gets:
    - `Output(ctx, timeout, env, args...)`: null stdin, `CLOUDSDK_CORE_DISABLE_PROMPTS=1`, returns `StepError` with the last stderr line.
    - `Interactive(ctx, stdio, env, args...)`.
    - `Command(env, args...)`.
  - Keep the "already exists" tolerance on create. Delete unused `parseWarnings`.
  - Verify: fake-gcloud tests assert null stdin, prompt-disable env, the timeout kill naming the command, and the explicit config env.
- [ ] 2.5 Add a read-only local view of gcloud configurations.
  - Files: `pkg/gcloudconfig/gcloudconfig.go`.
  - `ActiveConfig(dir)` reads the `active_config` file. `Read(dir, name)` returns `{Exists, Account, Project}` via a tolerant INI reader.
  - Verify: fixture tests for missing dir, missing file, comments/whitespace, other sections, and a malformed file (error, no panic).
- [ ] 2.6 Delete `pkg/watcher` once 2.3 replaces its use.
  - Verify: `go build ./...` passes, and `grep -r watcher pkg cmd` finds nothing.

## 3. Structured manager API and CLI presenter (Phase 1)

- [ ] 3.1 Define result and warning types plus validation.
  - Files: `pkg/manager/results.go`, `pkg/manager/validate.go`.
  - Define `Warning` codes, `StepError`, the result structs, `ValidateAccountName`, and `ValidateProjectID` per account-management and interactive-tui specs.
  - Verify: validation table tests from test-plan.md layer 2.
- [ ] 3.2 Add health and overview.
  - Files: `pkg/manager/status.go`.
  - Implement `Overview()`, `Info()`, and `CheckCredentials(ctx)` per spec "Health status definitions" and design "Account health model". Add a session `AUTH REQUIRED` set and a `bytes.Equal` default-ADC indicator. Add the `Manager` mutex.
  - Verify: health tests incl. zero-invocation/mtime-unchanged assertions.
- [ ] 3.3 Move printing out of manager and add `pkg/present`.
  - Files: `pkg/present/text.go`, `cmd/list.go`, `cmd/active.go`, `cmd/info.go`.
  - Move list/active/info output verbatim into the presenter. `list` now prints sorted (spec change 1).
  - Verify: characterization goldens unchanged except the `list` ordering test, which switches to exact order in this commit.
- [ ] 3.4 Convert switch.
  - Files: `pkg/manager/operations.go`, `cmd/switch.go`, `cmd/active.go`.
  - `Switch(ctx, name)` returns warnings. It sets the project with explicit `CLOUDSDK_ACTIVE_CONFIG_NAME`, and returns an error if persisting the active account fails. The presenter prints the historical warning text.
  - Verify: switch goldens unchanged; new tests for parent env pinning another config, activation failure, and `reauth-required`.
- [ ] 3.5 Convert save.
  - Files: `pkg/manager/operations.go`, `cmd/save.go`.
  - Read the email via `config get-value account` with the account's config env.
  - Verify: golden unchanged; test that the email comes from the account's own config.
- [ ] 3.6 Convert delete.
  - Files: `pkg/manager/operations.go`, `cmd/delete.go`.
  - Use bounded ADC removal of both canonical and recorded paths, and report each step's outcome. New warnings go to stderr.
  - Verify: golden stdout unchanged; tests for tampered `adc_path`, config-delete failure, and stale-credential non-resurrection.
- [ ] 3.7 Convert create and login.
  - Files: `pkg/manager/operations.go`, `pkg/present/text.go`, `cmd/create.go`, `cmd/login.go`.
  - Create does validation → duplicate check → config steps, then stops. Login takes `LoginIO` with ordered `Progress` events. It snapshots ADC before the ADC step, does not wait 5 s, and records the email from the account config.
  - Verify: create/login goldens identical (without the `Watcher warning` line, spec change 4). The tests drop the `testing.Short()` skip and now finish in < 2 s.
- [ ] 3.8 Return commands for run and shell.
  - Files: `pkg/manager/isolate.go`, `cmd/run.go`, `cmd/shell.go`.
  - `RunCommand` and `ShellCommand` return `*exec.Cmd`; the CLI wires stdio and runs it. Add `ResolveShell` (pure), including the Windows unresolvable-`SHELL` fallback.
  - Verify: run/shell goldens unchanged; shell-resolution table test; env assertions.
- [ ] 3.9 Phase-1 regression gate.
  - Verify: `grep -n 'fmt.Print' pkg/manager` returns nothing; the full characterization suite passes on 3 OSes; `go test -race ./...` passes on Linux.

## 4. Read-only TUI (Phase 2 — ships `gctx tui`, labelled experimental)

- [x] 4.1 Add dependencies.
  - Files: `go.mod`, `go.sum`.
  - Pin `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`, and `golang.org/x/term` per design D9 after re-checking release dates with the proxy (`.info` `Time` ≥ 7 days ago).
  - Verify: `go mod tidy -diff` empty; `go mod verify` passes; dates recorded in the PR.
- [x] 4.2 Add TTY detection and the `tui` command, without default routing.
  - Files: `cmd/tty.go`, `cmd/tui.go`, `cmd/root.go`.
  - `shouldLaunchTUI` is pure. Add `const tuiDefaultEnabled = false`. `gctx tui [--debug]` (short help: `Interactive terminal UI (experimental)`) uses `SilenceUsage`/`SilenceErrors`, with the non-TTY error and MinTTY hint.
  - Verify: routing table test; CLI tests for `gctx tui </dev/null` and `gctx tui x`; root-help golden updated with the `tui` line (spec change 3).
- [x] 4.3 Add the TUI skeleton.
  - Files: `pkg/tui/run.go`, `pkg/tui/model.go`, `pkg/tui/keys.go`, `pkg/tui/styles.go`.
  - Add the `Service` interface, the `Model` with the fields from design, `Init` (sync `Overview`, then the credential check cmd), quit handling, alt screen, focus reporting, and error mapping in `Run`.
  - Verify: a model test shows quitting returns `tea.Quit`; a PTY smoke test (test-plan 6.1) exits 0 with the restore sequences.
- [x] 4.4 Add the account table and header.
  - Files: `pkg/tui/view.go`.
  - Implement columns, marker, sorting, truncation, the `NO_COLOR`/`GCTX_ASCII` glyph sets, all header states, and scrolling.
  - Verify: model tests for the active-highlight, no-active, and drift-header scenarios; `G` with 40 rows.
- [x] 4.5 Add the responsive layout.
  - Files: `pkg/tui/view.go`.
  - Implement the breakpoints, too-small screen, and details pane at ≥ 120.
  - Verify: width/height boundary tests (59/60/89/90/119/120 × 11/12), 3 stripped goldens, and a random-resize no-panic test.
- [x] 4.6 Add navigation bindings.
  - Files: `pkg/tui/keys.go`, `pkg/tui/model.go`.
  - Implement `j/k/↑/↓/g/G/home/end/pgup/pgdn/ctrl+f/ctrl+b`, the `:` reserved message, and `No account selected`.
  - Verify: binding tests including `"G"` as text and no wrap.
- [x] 4.7 Add filtering.
  - Files: `pkg/tui/model.go`, `pkg/tui/view.go`.
  - Verify: all filter scenarios in the interactive-tui spec plus E2E-09 (A).
- [x] 4.8 Add the details view and help screen.
  - Files: `pkg/tui/view.go`, `pkg/tui/keys.go`.
  - Show findings with remedies. Generate help from the binding table. `q` closes overlays.
  - Verify: details scenario test; help lists every binding (test iterates the key table).
- [x] 4.9 Add credential check, reload, empty state, and error state.
  - Files: `pkg/tui/model.go`.
  - Implement `R`, the focus reload, seq-guarded `credsCheckedMsg`, `gcloud: not found`, empty and corrupt-config states, and status-message expiry.
  - Verify: tests for input responsiveness during a blocked fake check, stale-result drop, empty state, corrupt config unchanged, and the info expiry/error persistence clock tests.
- [x] 4.10 Add the performance guard.
  - Files: `pkg/manager/status_bench_test.go`, `pkg/tui/model_test.go`.
  - Add `BenchmarkOverview50`; a < 100 ms single-run assertion; and a check that no gcloud invocation is recorded before the first `View()` (E2E harness).
  - Verify: tests pass in CI.
- [x] 4.11 Security assertions for the read-only TUI.
  - Files: `pkg/tui/security_test.go`.
  - Assert seeded ADC marker strings never appear in any rendered screen. Assert `go list -deps ./pkg/tui` excludes clipboard packages.
  - Verify: tests pass.

## 5. Mutating actions (Phase 3)

- [x] 5.1 Add switch on `enter`.
  - Files: `pkg/tui/model.go`.
  - Run the background op with a spinner, warning-to-message mapping, re-apply on the active account, and the busy guard.
  - Verify: switch scenarios plus E2E-02 and E2E-03 (A with real Manager + fake gcloud).
- [x] 5.2 Add operation concurrency.
  - Files: `pkg/tui/model.go`.
  - Implement the single op, `Busy:` message, `q` while busy, double `ctrl+c` within 3 s (fake clock), and the stale-seq drop.
  - Verify: concurrency scenario tests.
- [x] 5.3 Add the save-ADC dialog.
  - Files: `pkg/tui/modals.go`.
  - Initial focus on `[Cancel]`. Show the default ADC path, mtime, and email. Show the missing-default-ADC message instead when the file is absent.
  - Verify: save scenarios and credential-safety "Cancel save".
- [x] 5.4 Add the delete dialog.
  - Files: `pkg/tui/modals.go`.
  - Show the fixed ADC notice, the checkbox (disabled for gcloud's active config), and focus handling. Select the nearest row after delete.
  - Verify: delete scenarios incl. `ctrl+d`→`enter` no-op; E2E-08 (A).

## 6. Login and create (Phase 4)

- [x] 6.1 Add the exec seam and login.
  - Files: `pkg/tui/exec.go`, `pkg/tui/model.go`.
  - `loginExec` implements `tea.ExecCommand`: banner, `svc.Login` with the presenter progress, Enter wait on failure. Add the `Model.exec` seam; `execDoneMsg` handling reloads, clears session auth, re-checks creds, and sets messages.
  - Verify: E2E-04 and E2E-04b (A with `execSync`); interactive-tui login scenarios.
- [x] 6.2 Add the create form.
  - Files: `pkg/tui/modals.go`.
  - Two `textinput`s with field navigation, validation messages, and duplicate and reused-config confirmation. Submit runs a background `Create`, then chains to `loginExec`. Select the new account and show the outcome message (incl. auth not completed).
  - Verify: create scenarios; E2E-07 both variants (A).

## 7. Run and shell (Phase 4)

- [x] 7.1 Add the run-prompt tokenizer.
  - Files: `pkg/tui/tokenize.go`.
  - Implement the quoting rules, leading-`gcloud` strip, metacharacter rejection, and the Windows `.cmd`/`.bat` rejection list.
  - Verify: `tokenize_test.go` table incl. the three credential-safety scenarios.
- [x] 7.2 Add the run flow.
  - Files: `pkg/tui/modals.go`, `pkg/tui/exec.go`.
  - The prompt has the ADC-missing hint and in-memory history (50). `runExec` prints the banner, runs `svc.RunCommand` cmd, prints the exit line, and waits for Enter. Map the exit code to a message. Reload after.
  - Verify: E2E-05 (A) incl. active account and default ADC unchanged; history and empty-input tests.
- [x] 7.3 Add the shell flow.
  - Files: `pkg/tui/exec.go`.
  - `shellExec` prints the banner and runs `svc.ShellCommand`. It resumes immediately and shows the exit message with status. It works without gcloud.
  - Verify: E2E-06 (A) with the fake shell; PTY smoke test 6.3.
- [x] 7.4 Gate actions when gcloud is unavailable.
  - Files: `pkg/tui/model.go`.
  - Verify: every gated key shows the message; `s`, `a`, `d` work (spec "gcloud unavailable").

## 8. Hardening (Phase 5)

- [x] 8.1 Add debug logging.
  - Files: `pkg/tui/run.go`, `cmd/tui.go`.
  - `GCTX_DEBUG`/`--debug`, `os.UserCacheDir()/gctx/tui-debug.log`, `0600`, 1 MiB truncate. Log only at the op boundary; record the verb only for run.
  - Verify: test with a run of `secrets versions access latest --secret=db-pass` asserts the log contains `secrets` and not `db-pass`; mode check on POSIX.
- [ ] 8.2 Add PTY smoke tests 6.1–6.4.
  - Files: `cmd/pty_unix_test.go`.
  - Add `github.com/creack/pty` v1.1.24 as a test-only dependency (≥ 7 days old).
  - Verify: passes on Linux and macOS CI.
- [ ] 8.3 Run the Ctrl+C and terminal-restoration review.
  - Confirm no `os.Exit` or `log.Fatal` under `pkg/tui`, and that panics inside `Update` restore the terminal.
  - Verify: a test injects a panicking service call and asserts `Run` returns an error; `grep` check in CI script.
- [ ] 8.4 Run the Windows verification.
  - Complete manual matrix rows 1–11 on the three Windows shells plus Git Bash. Fix findings.
  - Verify: matrix checked in the phase-5 PR (E2E-12).

## 9. Default routing (Phase 5)

- [ ] 9.1 Enable bare `gctx` routing.
  - Files: `cmd/root.go`, `cmd/tui.go`.
  - Set `tuiDefaultEnabled = true`; drop "experimental" from `tui`'s short help.
  - Verify: routing tests; PTY smoke test 6.2 (TUI with a TTY, help with `CI=true`); bare `gctx` non-TTY golden still byte-identical.
- [ ] 9.2 Run the full regression and acceptance gate.
  - Verify: every item in test-plan.md "Acceptance criteria" is satisfied, and the requirement→test mapping table is in the PR description.

## 10. Documentation and release (Phase 5)

- [ ] 10.1 Update the README.
  - Files: `README.md`.
  - Add an "Interactive TUI" section near the top of Usage (launch rules, `gctx tui`, `GCTX_NO_TUI`, keybinding table, screenshot placeholder). Add a Windows notes subsection (Windows Terminal/conhost, MinTTY → `winpty`). Note Go ≥ 1.26 under From Source. Replace "What's New in v0.5.0" with the new release's notes, including the bare-`gctx` behavior change.
  - Verify: review checklist.
- [ ] 10.2 Update ReadTheDocs.
  - Files: `readthedocs/docs/tui.md` (new: usage, keybindings, health statuses with meanings and remedies, run tokenizer rules, troubleshooting: terminal too small, `TERM=dumb`, MinTTY, `NO_COLOR`/`GCTX_ASCII`, debug log location, restoring a broken terminal), `readthedocs/docs/architecture.md` (new package layout and diagram from design.md, removing `watcher`), `readthedocs/mkdocs.yml` (nav).
  - Run `task docs` to regenerate the CLI reference including `gctx_tui.md`.
  - Verify: `mkdocs build --strict` passes locally; `task docs` leaves no diff.
- [ ] 10.3 Update contributor docs.
  - Files: `CONTRIBUTING.md`.
  - Explain how to run tests (fake gcloud, `-short`), the golden update flag, and the manual matrix for TUI changes.
  - Verify: review.
- [ ] 10.4 Prepare the release.
  - The phase-5 merge commit is `feat: open interactive TUI when gctx runs without arguments`, with a body describing the behavior change and `GCTX_NO_TUI`. Confirm the GoReleaser snapshot (`goreleaser release --snapshot --clean`) builds all 6 targets with `CGO_ENABLED=0`. Record the binary size delta in the PR.
  - Verify: snapshot artifacts exist; size noted.
- [ ] 10.5 Archive the change.
  - After release, run `openspec archive add-interactive-tui` to promote the specs to `openspec/specs/`.
  - Verify: `openspec validate --strict --all` passes.
