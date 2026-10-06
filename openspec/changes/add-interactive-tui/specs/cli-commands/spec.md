## Purpose

Defines the compatibility contract of the `gctx` command-line interface: the observable behavior of every existing command (which must survive the domain/presentation refactor), how a bare `gctx` invocation is routed between help output and the interactive TUI, and the new `gctx tui` command.

## ADDED Requirements

### Requirement: Existing commands remain available
The commands `create`, `login`, `save`, `switch`, `list`, `active`, `delete`, `run`, `shell`, `info`, `completion`, `version`, and the hidden `docs` SHALL remain available with their current arguments, flags, help text, and examples. These include `delete --gcloud-config`, `run` with flag parsing disabled, and the optional account argument of `switch`, `shell`, and `active`. Using the TUI SHALL NOT be required for any operation.

#### Scenario: Command surface unchanged
- **WHEN** `gctx --help` is run
- **THEN** every command listed above (except hidden `docs`) appears with its current short description
- **AND** the only addition is the `tui` command

#### Scenario: Run passes flags through
- **WHEN** `gctx run work storage buckets list --limit=1` is executed
- **THEN** gcloud receives the arguments `storage buckets list --limit=1` unchanged

### Requirement: Stable CLI output
For every existing command, stdout SHALL be byte-identical to the pre-change release, except for the explicit changes listed in this requirement. The exit code SHALL be 0 on success and 1 on error. The current error output on stderr (`Error: <message>`, usage, and the second `Error: <message>` line printed by `main`) SHALL be preserved. New diagnostics introduced by this change SHALL be written to stderr only. The explicit stdout changes are:
1. `list` orders accounts by name (previously in random map order).
2. `delete` reports failures that were previously ignored, on stderr.
3. Root help (`gctx`, `gctx --help`) gains one line for the `tui` command in its command list.
4. `create` and `login` no longer print `Watcher warning: timeout waiting for ADC file`, because the fixed wait is removed (see account-management "Login flow").

#### Scenario: list format
- **WHEN** accounts `work` (project `p1`, email `w@x.com`, active) and `alpha` (project `p2`, no email) exist and `gctx list` runs
- **THEN** stdout is exactly `\nConfigured Accounts:\n===================\n  alpha (p2)\n  work (p1) [w@x.com] ← active\n`

#### Scenario: list with no accounts
- **WHEN** no accounts exist and `gctx list` runs
- **THEN** stdout is exactly `No accounts configured\n` and the exit code is 0

#### Scenario: active output used by prompt scripts
- **WHEN** `work` is active and `GCTX_ACCOUNT` is unset
- **THEN** `gctx active` prints exactly `Active account: work\n`

#### Scenario: active inside an isolated shell
- **WHEN** `GCTX_ACCOUNT=personal` is set
- **THEN** `gctx active` prints `Active account: personal\n`

#### Scenario: switch with missing ADC
- **WHEN** `gctx switch sandbox` runs and `sandbox` has no stored ADC
- **THEN** stdout contains `\nWarning: No saved ADC credentials for account 'sandbox'.\nRun 'gctx login sandbox' to authenticate and save credentials.\n\n` followed by `Switched to account: sandbox (<project>)\n`
- **AND** the exit code is 0

#### Scenario: Unknown account
- **WHEN** `gctx info nope` runs and `nope` does not exist
- **THEN** the exit code is 1 and stderr contains `Error: account 'nope' not found`

### Requirement: No-argument routing
When `gctx` is invoked with no arguments at all, it SHALL launch the interactive TUI only if all of the following hold:
1. stdin is a terminal
2. stdout is a terminal
3. `TERM` is not `dumb`
4. `CI` is unset, empty, `false`, or `0`
5. `GCTX_NO_TUI` is unset or empty

Otherwise it SHALL print the root help to stdout and exit 0, byte-identical to the pre-change release. Any argument, including `--help`, `-h`, or `help`, SHALL bypass TUI routing. This routing SHALL be enabled only in the release that completes the TUI phases. Before that, bare `gctx` SHALL print help in all environments.

#### Scenario: Interactive terminal
- **WHEN** a user runs `gctx` in an interactive terminal with `TERM=xterm-256color` and no `CI`
- **THEN** the TUI opens

#### Scenario: Piped stdin
- **WHEN** `echo | gctx` runs
- **THEN** root help is printed to stdout, nothing waits for input, and the exit code is 0

#### Scenario: Stdout redirected
- **WHEN** `gctx > out.txt` runs from an interactive terminal
- **THEN** `out.txt` contains the root help and the exit code is 0

#### Scenario: CI environment
- **WHEN** `CI=true gctx` runs inside a pseudo-terminal
- **THEN** root help is printed and the TUI is not started

#### Scenario: Dumb terminal
- **WHEN** `TERM=dumb gctx` runs
- **THEN** root help is printed and the TUI is not started

#### Scenario: Explicit opt-out
- **WHEN** `GCTX_NO_TUI=1 gctx` runs in an interactive terminal
- **THEN** root help is printed

#### Scenario: Help flag
- **WHEN** `gctx --help` runs in an interactive terminal
- **THEN** root help is printed and the TUI is not started

### Requirement: tui command
`gctx tui` SHALL start the TUI whenever stdin and stdout are terminals and `TERM` is not `dumb`, regardless of `CI` or `GCTX_NO_TUI`. Otherwise it SHALL exit 1 with `Error: gctx tui requires an interactive terminal (stdin and stdout must be a TTY)`. On Windows, when the shell environment indicates a MinTTY session (`MSYSTEM` or `TERM_PROGRAM=mintty` set), the error SHALL additionally suggest running from Windows Terminal or prefixing with `winpty`. It SHALL accept `--debug` (equivalent to `GCTX_DEBUG=1`). It SHALL accept no positional arguments.

#### Scenario: Not a terminal
- **WHEN** `gctx tui < /dev/null` runs
- **THEN** it exits 1 with the terminal-required error and no escape sequences are written to stdout

#### Scenario: Positional argument
- **WHEN** `gctx tui work` runs
- **THEN** it exits 1 with cobra's argument error

### Requirement: TUI startup failure
If the TUI fails to initialize or exits with an internal error, the terminal SHALL be restored (cooked mode, main screen, cursor visible). Then `Error: interactive UI failed: <reason>` and `Run 'gctx --help' for available commands.` SHALL be printed to stderr, and the process SHALL exit 1.

#### Scenario: Initialization error
- **WHEN** the terminal cannot be put into raw mode
- **THEN** the shell prompt works normally afterwards and stderr shows the error and the help hint

### Requirement: Legacy interactive pickers unchanged
`gctx switch` and `gctx shell` without an account argument SHALL keep using the existing fuzzy-finder picker. They SHALL behave as today, including failing with `no accounts configured` when no accounts exist.

#### Scenario: Switch picker
- **WHEN** `gctx switch` runs with no argument in a terminal with accounts configured
- **THEN** the fuzzy finder lists `<name> [<project>]` entries with ` (active)` on the active one
