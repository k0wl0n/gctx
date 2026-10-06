## Why

Day-to-day use of `gctx` is a loop of `gctx list` → read → `gctx switch <name>` / `gctx login <name>` / `gctx run <name> ...`, with no single place that shows which accounts are healthy (ADC saved? gcloud configuration present? project drifted?). Users who manage several accounts want the fast, keyboard-driven "see everything, act on the selection" workflow that k9s gives Kubernetes users, without giving up the scriptable CLI.

This can't be added cleanly today: `pkg/manager` mixes business logic with `fmt.Printf` output, swallows several errors, and can only be exercised against a real `gcloud` and the real home directory. The repository has **zero tests**. A TUI built on top of that would either parse CLI output or duplicate logic. So this change first separates domain operations from presentation, then builds a thin TUI on top of the shared, tested operations.

## What Changes

- **Domain/presentation split (no user-visible change):** `pkg/manager` operations return structured results (account status, typed warnings, per-step outcomes) instead of printing. A text presenter in the CLI layer reproduces today's output byte-for-byte. Filesystem roots and the `gcloud` binary become injectable so the logic can be tested hermetically.
- **Account health model:** a local, fast, side-effect-free status for every account (`READY`, `ADC MISSING`, `CONFIG MISSING`, `PROJECT MISMATCH`, `AUTH REQUIRED`, `ERROR`) plus an asynchronous, network-free credential check.
- **New `gctx tui` command:** a full-screen, k9s-inspired interface for listing, filtering, inspecting, switching, creating, deleting, logging in, saving ADC, running isolated `gcloud` commands, and opening isolated shells. Interactive subprocesses (browser login, shell, run) suspend the TUI and hand the terminal over, then resume it.
- **Bare `gctx` launches the TUI** only when stdin and stdout are both terminals and the environment is not CI/`TERM=dumb`/`GCTX_NO_TUI`. Otherwise it prints help exactly as today. This routing is enabled in the final phase, after the TUI is feature-complete. **Behavior change for interactive users** who ran bare `gctx` to see help (they now use `gctx --help`). Scripts and pipes are unaffected.
- **Bug fixes the TUI depends on** (each called out in specs/design):
  - `create` checks for a duplicate account before making any gcloud change.
  - Account names are validated against gcloud's configuration-name rules before any side effect.
  - Login no longer waits 5 seconds for the ADC watcher on every run.
  - `save` reads the email from the account's own gcloud configuration, not whichever configuration is globally active.
  - `delete` reports failures instead of silently ignoring them, and only removes ADC files inside gctx's own ADC directory.
  - Config writes are atomic, and every mutation re-reads the file first.
  - Account-scoped gcloud calls target the account's configuration explicitly. Today, `gctx switch` run from inside `gctx shell` sets the project on the wrong configuration.
  - The default ADC location honors `CLOUDSDK_CONFIG`, like gcloud does.
  - On Windows, an unresolvable `$SHELL` (for example Git Bash's `/usr/bin/bash`) falls back to the PowerShell/cmd chain instead of failing.
- **Toolchain/dependencies:** add Bubble Tea v2, Bubbles v2, and Lip Gloss v2, with pinned versions at least 7 days old. Promote `golang.org/x/term` to a direct dependency. Raise `go` in `go.mod` from the end-of-life 1.24 to 1.26. Fix the untidy `go.mod`, where `go-fuzzyfinder` is incorrectly marked indirect.
- **CI:** run tests on Linux, macOS, and Windows. Pin actions by SHA. Add `dependabot.yml` with a 7-day cooldown.
- **Docs:** README, ReadTheDocs (new TUI page, keybindings, troubleshooting, Windows notes), regenerated CLI reference, and a refreshed `architecture.md`.

## Capabilities

### New Capabilities

- `account-management`: account lifecycle and health semantics shared by CLI and TUI. Covers status definitions, validation, switch/login/save/delete outcomes, isolated execution environment, and config persistence safety.
- `cli-commands`: compatibility contract for every existing command, plus no-argument routing and the new `gctx tui` command.
- `interactive-tui`: observable behavior of the TUI. Covers layout, navigation, keybindings, filtering, modals, async operations, suspend/resume, resize, empty state, and errors.
- `credential-safety`: rules for how credentials and account metadata may be displayed, logged, stored, deleted, and passed to subprocesses.

### Modified Capabilities

None. `openspec/specs/` is empty (OpenSpec is initialized by this change), so the existing CLI behavior is captured as the new `cli-commands` baseline.

## Impact

- **Code:** `cmd/*` (presenter, `root.go` routing, new `tui.go`), `pkg/manager` (structured API), `pkg/config` (atomic save, injectable path), `pkg/adc` and `pkg/gcloud` (injectable roots and binary), `pkg/watcher` (pre-login snapshot). New packages: `pkg/tui` and `pkg/gcloudconfig` (local read-only view of gcloud configurations). New test-only helper: `internal/testutil/fakegcloud`.
- **Dependencies:** `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, and `charm.land/lipgloss/v2`, plus their transitive packages, which roughly doubles the module count. Binary size grows by an estimated 2–4 MB. `go-fuzzyfinder` is kept for the legacy pickers in this change.
- **Toolchain:** the minimum Go version to build from source becomes 1.26. `go install` users on older toolchains get Go 1.26 automatically through `GOTOOLCHAIN=auto` (the default).
- **Config schema:** unchanged (`config.json` fields and the ADC storage layout are untouched).
- **Security:** the TUI shows ADC *paths*, emails, and project IDs, but never ADC contents or tokens. Run input is executed as argv with no shell. Details are in `specs/credential-safety`.
- **Cross-platform:** TTY detection, terminal suspend/resume, and shell resolution must behave on Windows Terminal, conhost, PowerShell, and cmd. Git Bash/mintty without winpty falls back to help. Details are in design.md.
- **Risks:** the refactor could regress CLI output. This is mitigated by characterization tests written *before* the refactor. Also, terminal state could be left corrupted on Windows if a subprocess crashes. This is mitigated by the framework's restore-on-exit/panic behavior and a manual verification matrix.
