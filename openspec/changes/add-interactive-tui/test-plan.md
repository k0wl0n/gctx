# Test Plan: add-interactive-tui

This plan supplements `tasks.md`. Each layer names its location, the tools it uses, and which spec requirements it covers. All automated tests run with `go test ./...` (`task test`) on `ubuntu-latest`, `macos-latest`, and `windows-latest`, unless marked otherwise.

## Test infrastructure

| Component | Location | Purpose |
|---|---|---|
| Fake gcloud | `internal/testutil/fakegcloud/main.go` | Built once per test binary in `TestMain` (`go build -o <tmp>/gcloud[.exe]`). It emulates the gcloud verbs gctx uses by manipulating `$CLOUDSDK_CONFIG`: `configurations create/activate/delete/list`, `config set project`, `config get-value account`, `auth list --format=json`, `auth login`, `auth application-default login`. It appends a JSONL record per call to `$FAKE_GCLOUD_LOG`, containing `argv`, `CLOUDSDK_ACTIVE_CONFIG_NAME`, `CLOUDSDK_CORE_DISABLE_PROMPTS`, and whether stdin was null. Failures are injected with `FAKE_GCLOUD_FAIL=<verb>:<exit>[:<stderr>]`, latency with `FAKE_GCLOUD_SLEEP=<verb>:<ms>`, and `FAKE_GCLOUD_ADC_UNCHANGED=1` makes ADC login leave the file untouched. |
| Harness | `internal/testutil/harness.go` | `NewEnv(t)` creates a temp root and sets `HOME`, `USERPROFILE`, `APPDATA`, `CLOUDSDK_CONFIG`, and `PATH` (fake gcloud first) with `t.Setenv`. It returns `manager.Paths`, a `*gcloud.Client`, plus helpers: `SeedAccount(name, project, withADC, withConfig)`, `Invocations()`, `ReadConfig()`, `FileMode(path)`. |
| CLI runner | `cmd/cli_test.go` | Builds the real `gctx` binary once (`go build -o <tmp>/gctx`). Runs it with the harness env and captures stdout, stderr, and the exit code separately. |
| TUI driver | `pkg/tui/testing_test.go` | `newTestModel(svc, w, h)`, `press(m, keys...)`, `resize(m, w, h)`, and `drain(m, cmd)`. `drain` executes returned `tea.Cmd`s inline, including batches/sequences, and feeds the messages back until quiescent, with a step limit. `screen(m)` returns `ansi.Strip(m.View().Content)`. `execSync` replaces `Model.exec`: it runs the `ExecCommand` against in-memory pipes with scripted stdin (for example `"\n"`) and returns the callback message. |
| Fake service | `pkg/tui/fakeservice_test.go` | In-memory `tui.Service`. It returns scripted `Overview` values and results, records calls, and can block until released (for concurrency tests). |
| PTY smoke | `cmd/pty_unix_test.go` (`//go:build !windows`) | Uses `github.com/creack/pty` v1.1.24 (test-only) to run the built binary in a real pseudo-terminal. |

## Layers and coverage

### 1. Characterization tests (written first, against the unmodified code)

`cmd/cli_test.go`. These freeze current behavior before any refactor. One table-driven case per command:

| Command | Assertions |
|---|---|
| `list` (0, 1, 3 accounts) | exact stdout bytes; exit 0. With 3 accounts the current order is random, so the pre-refactor test compares a **sorted set of lines**. After task 3.3 it asserts the exact sorted order (spec change 1). |
| `active` (config / `GCTX_ACCOUNT` / none) | `Active account: <x>\n` |
| `active <name>` | identical to `switch` output |
| `switch` (with ADC / without ADC / set-project reauth failure / activate failure) | exact stdout; fake-gcloud invocation sequence; resulting `config.json`; default ADC bytes |
| `create` (success / duplicate / set-project fails / auth login fails / ADC login fails) | stdout line sequence. Fake gcloud records. Because of the existing 5 s watcher, the pre-refactor create/login tests are tagged `slow` and run in CI only. |
| `login` | as create, minus configuration creation |
| `save` (ok / no default ADC / unknown account) | stdout; stored file bytes; mode `0600` on POSIX |
| `delete` (with/without `--gcloud-config`) | stdout `Deleted account: x\n`; files removed; config entry removed |
| `run` (exit 0 / exit 3 / flags passthrough) | gcloud argv; env contains isolation vars; gctx exit code 1 on failure with `Error: exit status 3` |
| `shell` | uses `SHELL=<fake-shell>`, where the fake shell is the fake gcloud binary in "env dump" mode; env vars present; banner text |
| `info` | exact stdout |
| `completion bash/zsh/fish/foo` | non-empty output; `foo` → exit 1, `unsupported shell: foo` |
| `version` | 5 lines, prefixes stable |
| bare `gctx` (non-TTY) | stdout equals the recorded help golden `testdata/root-help.golden`; exit 0 |
| `gctx --help` | same golden (plus the `tui` line after task group 4 — golden updated in that commit) |
| unknown command / unknown account | exit 1; stderr contains the existing double `Error:` lines |

Goldens live in `cmd/testdata/`. They are refreshed only with `go test ./cmd -update`, and every golden diff must be justified by a spec-listed change in the commit message.

### 2. Domain unit tests (`pkg/manager`, `pkg/config`, `pkg/adc`, `pkg/gcloud`, `pkg/gcloudconfig`)

| Area | Cases |
|---|---|
| Health mapping | one test per status; priority with combined findings; corrupt ADC → `ERROR` without file bytes in the message; corrupt gcloud config file → `ERROR`; `CLOUDSDK_CONFIG` respected; missing gcloud dir → `CONFIG MISSING` for all |
| No side effects | `Overview()` records zero fake-gcloud invocations and leaves mtimes of all inputs unchanged |
| Credential check | uncredentialed → `AUTH REQUIRED`; gcloud missing → warning plus unchanged statuses; timeout via `FAKE_GCLOUD_SLEEP` with a short test timeout |
| Validation | `ValidateAccountName` table (valid: `a`, `work-2`; invalid: ``, `Work`, `1a`, `a b`, `../x`, `a/b`, `a\b`, `a.b`); `ValidateProjectID` table (valid: `my-proj-1`, `example.com:my-proj`; invalid: `ab`, `My-proj`, `proj-`, 31 chars) |
| Create | duplicate checked before any gcloud call (zero invocations); invalid name → zero invocations; `config-reused` warning; set-project failure is a warning |
| Login | step errors identify `auth login` / `application-default login`; stored ADC untouched on failure; no fixed wait (test finishes in < 1 s); `adc-not-refreshed` when unchanged; email read with `CLOUDSDK_ACTIVE_CONFIG_NAME=<acct>-config` |
| Switch | warnings `adc-missing`, `reauth-required`; activation failure leaves active unchanged; project set with explicit config env even when the parent env pins another config |
| Save | email from the account's own config while another config is globally active |
| Delete | ADC removed; config kept by default; `config-delete-failed` still removes the account; `adc_path` outside store not deleted, plus warning; default ADC never deleted |
| Isolation | `RunCommand`/`ShellCommand` env contents; global state byte-identical before/after running the returned cmd |
| Shell resolution | pure-function table over `goos`, `SHELL`, a `lookPath` stub (pwsh/powershell/COMSPEC/cmd/unresolvable SHELL on Windows; `$SHELL`, `/bin/sh` on Unix) |
| Non-interactive gcloud | stdin null, `CLOUDSDK_CORE_DISABLE_PROMPTS=1` (from the fake log); timeout kills the process and the error names the command |
| Persistence | atomic save (no `.tmp` left behind; previous content intact when rename is forced to fail); re-read-before-write preserves an account added externally; schema round-trip equality with a fixture of the current format |
| Permissions (POSIX only) | stored ADC `0600`, ADC dir `0700`, restored default ADC `0600` |
| Benchmark | `BenchmarkOverview50` (50 seeded accounts) must stay < 20 ms/op on CI Linux; a test asserts a single run < 100 ms |

### 3. TUI model tests (`pkg/tui`, fake service)

Behavior assertions on model state and stripped view text. No full-screen goldens except the three breakpoint goldens.

| Area | Cases |
|---|---|
| Keybindings | every binding in the spec table dispatches the expected action; `G` arrives as text `"G"`; `d` opens details (never delete); `:` message; no wrap at bounds; page up/down |
| Selection | initial selection = active account or first row; preserved by name across reload, filter, and resize; nearest-row after delete; empty list |
| Filter | substring across name/project/email/config; case-insensitive; `j` typed into query; `↑/↓` move; `enter` keeps; `esc` clears (editing vs applied); zero-match text; title count |
| Layout | widths 59/60/89/90/119/120 and heights 11/12. Asserts the column set, details pane presence, too-small message, and selected row visible after `G` with 40 accounts. One stripped golden at 60×20, 100×20, 130×30. Rapid resize sequence of 100 random sizes does not panic. |
| Header | drift `⚠`; `Shell: <name>`; ADC indicator states; `gcloud: not found` |
| Modals | create (field nav, each validation message, duplicate, reused-config second-enter, esc no service call); delete (initial focus `[Cancel]`, `ctrl+d` then `enter` deletes nothing, checkbox toggle, disabled checkbox for gcloud's active config); save (default ADC missing → message instead of dialog; cancel → no call) |
| Operations | switch success/warnings/error messages; busy guard (`Busy: …`); stale result ignored (seq); `q` while busy; double `ctrl+c` within 3 s quits, after 3 s it re-arms (fake clock) |
| Status messages | info expires after 5 s (fake clock + `statusExpiredMsg`); warnings/errors persist |
| Credential check | runs after the first frame (no service call during `Init`'s synchronous part); spinner text; merge result; failure → single warning |
| Exec flows (with `execSync`) | login success/cancel/ADC-fail messages and reload; run banner, exit line, Enter wait, history recall (↑/↓, cap 50), empty input not submitted, tokenizer errors inline, ADC-missing hint; shell exit message incl. non-zero status |
| gcloud unavailable | each gated key shows the message; `s`, `a`, `d` still work |
| Empty/error states | empty text; `c` from empty; corrupt config error state; mutations disabled; config bytes unchanged |
| Security | view never contains seeded ADC JSON (`refresh_token` marker string) in any screen/modal; no clipboard package imported (`go list -deps ./pkg/tui` excludes `github.com/atotto/clipboard`, asserted in a test) |

`tokenize_test.go` is a table test for the run-prompt tokenizer, metacharacter rejection, and the Windows `.cmd` rejection list.

### 4. Integration tests (`pkg/tui`, real `Manager` + fake gcloud + harness)

These are the automated form of the E2E scenarios below. The model is wired to a real `*manager.Manager` over the harness. Subprocess flows use `execSync`, so ExecCommands really run fake gcloud, or the fake shell, against pipes.

### 5. CLI regression (after refactor)

Layer 1 re-run unchanged, plus:
- routing table test for `shouldLaunchTUI` (all spec conditions; `CI` values `true`, `1`, `false`, `0`, empty)
- `gctx tui </dev/null` → exit 1 + message, no ESC bytes on stdout
- `gctx tui extra` → exit 1
- MinTTY hint when `MSYSTEM` is set (Windows-only test)

### 6. PTY smoke tests (Linux + macOS CI)

`cmd/pty_unix_test.go`, run with the built binary in a PTY at 120×40:
1. `gctx tui` with 3 seeded accounts. Wait for `ACCOUNTS`/`NAME` in output. Send `q`. Expect exit 0, and an output stream that ends with the alt-screen exit sequence `ESC[?1049l` and cursor-show `ESC[?25h`.
2. Bare `gctx` with `tuiDefaultEnabled` (after task group 9): same as above. With `CI=true`: help text, no `ESC[?1049h`.
3. `s` with `SHELL` set to the fake shell in "print env and exit" mode. The output contains `GCTX_ACCOUNT=<name>`. The TUI redraws (the `NAME` header appears again after the shell output). Then `q` → exit 0.
4. Resize the PTY (`pty.Setsize`) 20 times between 40×10 and 200×60, then `q`. Exit 0, no `panic` in output.

Windows has no PTY automation in CI. Its equivalents are in the manual matrix.

## E2E scenarios

**Legend:**
- **A** = automated integration test (layer 4, all OSes)
- **P** = PTY smoke (Linux/macOS)
- **M** = manual verification (checklist in the PR / release)

| ID | Scenario | Steps | Expected | Coverage |
|---|---|---|---|---|
| E2E-01 | Launch TUI | Seed `personal`, `sandbox`, `work` (active). Run `gctx` (after routing flip) or `gctx tui`. | TUI opens. `work` row has `●` and is selected. Project and email are visible. Header shows `Active: work (…)`. `q` exits 0 and the terminal is restored. | A, P, M |
| E2E-02 | Switch | Select `sandbox`, press `enter`. | Fake log shows `configurations activate sandbox-config` and `config set project <p>` with `CLOUDSDK_ACTIVE_CONFIG_NAME=sandbox-config`. The default ADC equals sandbox's stored ADC. `config.json` has `active_account=sandbox`. `●` moves to sandbox and the header updates. | A, M |
| E2E-03 | Missing ADC | Seed `nocreds` without ADC. Select it, press `enter`. | gcloud config activated. Status line shows the no-saved-ADC warning suggesting `l`. Row status `ADC MISSING`. No crash. | A |
| E2E-04 | Login | Select `sandbox`, press `l`. | `execSync` runs loginExec. The fake log shows `auth login` then `auth application-default login`, both with sandbox's config env. The stored ADC is updated (`0600`). `adc_path` and email are recorded. Status line shows `Login complete for sandbox`. The credential check is re-run. **M**: real browser auth on each OS; TUI suspends visibly and resumes. | A, M |
| E2E-04b | Login cancelled | `FAKE_GCLOUD_FAIL=auth-login:1`. | `Press Enter to return to gctx` written to the pipe. After the scripted Enter: `Login cancelled for sandbox; credentials unchanged`. Stored ADC bytes unchanged. | A |
| E2E-05 | Run isolated | `work` active. Select `personal`, press `r`, enter `gcloud config get-value project`. | Fake log shows argv `config get-value project` with `CLOUDSDK_ACTIVE_CONFIG_NAME=personal-config` and `GOOGLE_APPLICATION_CREDENTIALS=<personal stored ADC>`. Pipe output contains the banner and `exit status 0`. After Enter, `config.json` active is still `work` and the default ADC is byte-identical. | A, M |
| E2E-06 | Shell | Select `personal`, press `s` (fake shell dumps env and exits 0). | Env contains `GCTX_ACCOUNT=personal`, `CLOUDSDK_ACTIVE_CONFIG_NAME=personal-config`, `GOOGLE_APPLICATION_CREDENTIALS`. Status `Shell for personal exited`. Selection unchanged. **P** run with a real PTY. **M**: real `$SHELL` (zsh/bash/fish) and Windows shells. | A, P, M |
| E2E-07 | Create | Press `c`, enter `demo` / `demo-project`, submit. | Fake log: `configurations create demo-config`, `activate`, `config set project demo-project`, then (via execSync) `auth login`, `auth application-default login`. `demo` is listed and selected. Variant with `FAKE_GCLOUD_FAIL=auth-login:1`: `demo` is listed with `AUTH REQUIRED`/`ADC MISSING` and the retry message. | A, M |
| E2E-08 | Delete | (a) `ctrl+d` on `sandbox` → focus `[Delete]` → `enter`. (b) Recreate sandbox. `ctrl+d` → toggle checkbox → `[Delete]`. | (a) sandbox gone from config, stored ADC removed, `config_sandbox-config` still exists, no `configurations delete` in the log. (b) log contains `configurations delete sandbox-config --quiet` and the file is gone. Variant: deleting gcloud's active config has the checkbox disabled. | A |
| E2E-09 | Search | `/sandbox` then `enter`. | Only matching rows. Title `filter: sandbox (1/3)`. Selection on `sandbox`. `esc` restores all 3 rows with `sandbox` still selected. | A |
| E2E-10 | Resize | 100 random resizes incl. below the minimum (A). 20 PTY resizes (P). Manual window drag (M). | No panic. Breakpoint layouts as specified. Selection and filter intact. | A, P, M |
| E2E-11 | Ctrl+C | (a) Main view → exit 0. (b) During login (`FAKE_GCLOUD_SLEEP=auth-login:…`, M: real) → gcloud interrupted, Enter prompt, TUI resumes with the cancel message. (c) During run → exit line printed, TUI resumes. (d) In a shell → handled by the shell (prompt redraws), `exit` returns to the TUI. (e) During an in-flight switch → first press warns, second within 3 s quits. | as described | A (a, e), P (a), M (b–d all OSes) |
| E2E-12 | Windows | On Windows 11 run the manual matrix below for PowerShell 7, Windows PowerShell 5.1, and cmd.exe, each under Windows Terminal and conhost. | All pass. Terminal restored (cursor visible, echo on, no stray escape codes) after `q`, after `s`→`exit`, after `r`, after `l` cancel. | M (+ A for shell resolution and `.cmd` safety) |

## Manual verification matrix (attached to the phase-5 PR and repeated for the release)

The checklist is run for each environment:
- macOS (Terminal.app, iTerm2; zsh)
- Linux (GNOME Terminal or Alacritty; bash; plus a tmux session)
- Windows 11 (Windows Terminal + PowerShell 7; Windows Terminal + Windows PowerShell 5.1; conhost + cmd.exe; Git Bash/MinTTY)

| # | Check |
|---|---|
| 1 | `gctx` opens the TUI. `echo | gctx` and `gctx > f.txt` print help. `CI=1 gctx` prints help. `GCTX_NO_TUI=1 gctx` prints help. |
| 2 | Navigation, filter, details, help, and `q` work. After quitting: prompt on the normal screen, scrollback preserved, echo on. |
| 3 | `l` with real browser auth for a test account: TUI suspends, both browser flows complete, TUI resumes, ADC ✓. |
| 4 | `l` then `ctrl+c` at the browser prompt: Enter prompt, cancel message, terminal sane. |
| 5 | `r` `projects list --limit=1`: output readable, exit line, Enter returns. `r` then `ctrl+c` during a slow command (`compute instances list` on a large project). |
| 6 | `s`: correct shell (Windows: pwsh → powershell → cmd per availability; Git Bash `SHELL` falls back on Windows); `gcloud config list` shows the account; `exit` returns. |
| 7 | `c` full create with real auth; `ctrl+d` delete of that account, both variants. |
| 8 | Resize by dragging the window to tiny and back; tmux pane split/zoom. |
| 9 | `NO_COLOR=1` and `GCTX_ASCII=1` render legibly. |
| 10 | Git Bash/MinTTY: `gctx` prints help; `gctx tui` prints the winpty/Windows Terminal hint; `winpty gctx tui` works. |
| 11 | `GCTX_DEBUG=1 gctx`: log file created with mode 0600 (POSIX), contains event lines, and contains no ADC JSON, tokens, or run arguments. |

## Acceptance criteria (release gate for phase 5)

1. `openspec validate add-interactive-tui --strict` passes.
2. `go test ./...` passes on Linux, macOS, and Windows in CI. `go vet ./...` is clean. `go mod tidy -diff` is empty.
3. Every requirement in `specs/*/spec.md` maps to at least one automated test, or to a numbered manual check. The mapping table is kept in the PR description. Missing mappings block release.
4. CLI characterization goldens are unchanged except for the four spec-listed stdout changes (each with its own commit).
5. The manual matrix is fully checked for all environments, with failures fixed or explicitly waived in the PR.
6. `BenchmarkOverview50` < 20 ms/op on CI Linux. The PTY smoke test reaches first frame in < 500 ms of process start on CI (a loose bound for CI noise; the 100 ms target is measured locally and recorded in the PR).
7. README, ReadTheDocs pages, and the generated CLI reference are updated (`task docs` produces no diff after commit).
