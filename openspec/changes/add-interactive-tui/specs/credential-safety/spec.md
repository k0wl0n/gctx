## Purpose

Defines how gctx handles credentials and account metadata across the CLI and the TUI: what may be displayed or logged, how credential files are stored, deleted, and passed to subprocesses, and how user-entered commands are executed without shell injection.

## ADDED Requirements

### Requirement: Credential contents are never exposed
The system SHALL NOT display, print, log, or copy to the clipboard any of the following:
- the contents of any ADC file (stored or default)
- any access token, refresh token, ID token, or client secret

The TUI and logs MAY show ADC file paths, file modification times, account names, emails, project IDs, and gcloud configuration names. Error messages derived from ADC parsing SHALL describe the problem without echoing file bytes.

#### Scenario: Invalid ADC error
- **WHEN** a stored ADC file contains `{"refresh_token": "1//abc` (truncated JSON)
- **THEN** the reported error is `saved ADC file is invalid: <path>: unexpected end of JSON input` or equivalent
- **AND** the text `1//abc` appears nowhere in the TUI, stdout, stderr, or debug log

#### Scenario: No token-printing commands
- **WHEN** any status check, health check, or TUI action runs
- **THEN** gctx never invokes `gcloud auth print-access-token`, `gcloud auth print-identity-token`, or `gcloud auth application-default print-access-token`

### Requirement: No clipboard access
The TUI SHALL NOT read or write the system clipboard.

#### Scenario: Copy keys unbound
- **WHEN** the user presses `ctrl+c`, `y`, or `c` in the TUI account view
- **THEN** nothing is written to the clipboard

### Requirement: Credential file permissions
Stored ADC files SHALL be created with mode `0600` and the stored ADC directory with mode `0700` (on POSIX systems). The default ADC file restored by switch SHALL be written with mode `0600` through a temporary file in the same directory followed by an atomic rename. `config.json` contains no secret material and SHALL keep its current permissions (`0644`, directory `0755`). On Windows, files SHALL inherit the user-profile ACLs; no broadening of ACLs is permitted.

#### Scenario: Saved ADC permissions
- **WHEN** login saves ADC for `work` on Linux or macOS
- **THEN** `~/.config/gctx/adc/work_adc.json` has mode `0600` and its directory has mode `0700`

### Requirement: Bounded credential deletion
Deleting an account SHALL remove its stored ADC file. Removal SHALL apply only to:
- the canonical storage path `<gctx dir>/adc/<name>_adc.json`
- the recorded `adc_path`, only when it resolves to a location inside `<gctx dir>/adc/`

A recorded `adc_path` outside that directory SHALL NOT be deleted and SHALL produce an `adc-path-outside-store` warning. The default ADC file SHALL never be deleted by gctx.

#### Scenario: Tampered adc_path
- **WHEN** `config.json` records `adc_path` as `/home/u/.ssh/id_ed25519` for account `x` and `x` is deleted
- **THEN** `/home/u/.ssh/id_ed25519` still exists
- **AND** the result contains an `adc-path-outside-store` warning

#### Scenario: Stale credentials not resurrected
- **WHEN** account `x` is deleted and a new account `x` is created later
- **THEN** no stored ADC exists for the new `x` until it logs in or saves

### Requirement: Save ADC requires explicit confirmation in the TUI
Because saving copies whatever credentials are currently in the default ADC location, the TUI save action SHALL show a confirmation that names the target account. It SHALL also show the email of the account's gcloud configuration when known. Nothing SHALL be written unless the user confirms.

#### Scenario: Cancel save
- **WHEN** the user presses `a` on `sandbox` and then `Esc`
- **THEN** `sandbox`'s stored ADC file is unchanged

### Requirement: User commands are executed without a shell
Commands entered in the TUI run prompt SHALL be split into arguments by gctx's own tokenizer and passed to `gcloud` as an argument vector. No shell interpreter SHALL process the input. The tokenizer works as follows:
- Whitespace separates arguments.
- `'...'` is literal.
- Inside `"..."`, a backslash escapes `"` and `\`.
- Outside quotes, a backslash escapes only whitespace, quotes, and backslash; any other backslash is literal.
- A leading `gcloud` token is dropped.

Input with an unterminated quote, or with an unquoted `|`, `;`, `&`, `>`, `<`, `` ` ``, or `$(`, SHALL be rejected with an inline message suggesting the isolated shell (`s`).

#### Scenario: Shell metacharacters rejected
- **WHEN** the user submits `compute instances list; rm -rf ~`
- **THEN** nothing is executed and the prompt shows `Shell operators are not supported here; press s for an isolated shell`

#### Scenario: Quoted filter preserved
- **WHEN** the user submits `compute instances list --filter="name~'^web' AND status=RUNNING"`
- **THEN** gcloud receives exactly 4 arguments, the last being `--filter=name~'^web' AND status=RUNNING`

#### Scenario: Windows path backslashes
- **WHEN** the user submits `storage cp C:\tmp\a.txt gs://b/`
- **THEN** the second argument is `C:\tmp\a.txt`

### Requirement: Windows batch-wrapper argument safety
On Windows, when the resolved `gcloud` executable is a `.cmd` or `.bat` file, the TUI run prompt SHALL reject any argument containing `&`, `|`, `<`, `>`, `^`, `%`, `!`, `"`, or a newline. The rejection message SHALL suggest the isolated shell. The CLI `gctx run` command SHALL keep its current behavior.

#### Scenario: Percent sign on Windows
- **WHEN** on Windows with `gcloud.cmd` the user submits `config set core/project %USERPROFILE%`
- **THEN** nothing is executed and the inline error explains the restriction

### Requirement: Debug logging is redacted and opt-in
Debug logging SHALL be disabled unless `GCTX_DEBUG=1` or `gctx tui --debug` is used. When enabled, it SHALL write to `<user cache dir>/gctx/tui-debug.log` with mode `0600`, truncating the file when it exceeds 1 MiB at startup. It SHALL record only:
- event names
- account names
- gcloud subcommand verbs (the first two arguments; never user-entered run arguments beyond the first word)
- durations
- exit codes
- error messages

It SHALL never write to the terminal while the TUI is active.

#### Scenario: Run arguments not logged
- **WHEN** debug logging is on and the user runs `secrets versions access latest --secret=db-pass`
- **THEN** the log records the run event with verb `secrets` and the exit code, and does not contain `db-pass`

### Requirement: No new secret storage or telemetry
This change SHALL NOT introduce:
- new files containing credentials
- persistent command history
- network calls other than those made by `gcloud` itself
- telemetry of any kind

TUI run history SHALL exist in memory only for the life of the TUI process.

#### Scenario: History not persisted
- **WHEN** the user runs commands in the TUI and quits
- **THEN** no file under the gctx config or cache directories contains those commands
