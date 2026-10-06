## Purpose

Defines the account lifecycle and account health semantics that both the CLI and the interactive TUI rely on: how account status is computed, how inputs are validated, what switch/login/save/delete/create guarantee, how isolated execution environments are built, and how account metadata is persisted safely.

## ADDED Requirements

### Requirement: Structured account operations
Every account operation (list, status, info, active, create, login, save, switch, delete, run, shell) SHALL be available as a structured operation. It returns data, typed warnings, and typed errors, and writes nothing to stdout or stderr itself. Human-readable output SHALL be produced only by a presenter (CLI text or TUI). No presenter SHALL obtain account data by parsing another presenter's output.

#### Scenario: Operation produces no direct output
- **WHEN** the switch operation runs for an account without saved ADC
- **THEN** it returns a result containing a warning of kind `adc-missing` for that account
- **AND** nothing is written to the process stdout or stderr by the operation itself

#### Scenario: TUI and CLI observe identical results
- **WHEN** the same account store is read by the CLI `list` command and by the TUI
- **THEN** both show the same set of account names, project IDs, emails, and active marker

### Requirement: Local account status
The system SHALL compute a status for every account using only local filesystem reads. It SHALL NOT start subprocesses, make network requests, or modify any file. The status inputs are:
1. the gctx account record
2. whether the account's stored ADC file exists and parses as a JSON object
3. whether the account's gcloud configuration exists in the local gcloud configuration directory
4. the `core/project` and `core/account` values recorded in that gcloud configuration

The gcloud configuration directory SHALL be `$CLOUDSDK_CONFIG` when set. Otherwise it is `%APPDATA%\gcloud` on Windows and `~/.config/gcloud` elsewhere.

#### Scenario: Status computation has no side effects
- **WHEN** statuses are computed for all accounts
- **THEN** no `gcloud` process is started
- **AND** the modification times of `config.json`, every stored ADC file, the default ADC file, and every gcloud configuration file are unchanged

#### Scenario: Status uses CLOUDSDK_CONFIG
- **WHEN** `CLOUDSDK_CONFIG` points to a directory containing `configurations/config_work-config`
- **THEN** the account whose configuration name is `work-config` is not reported as `CONFIG MISSING`

### Requirement: Health status definitions
Each account SHALL carry a list of all findings that apply to it. It SHALL also carry exactly one primary status, which is the highest-priority finding in this order:

| Priority | Status | Meaning |
|---|---|---|
| 1 | `ERROR` | A local check could not be completed (for example, permission denied reading a stored ADC file, a stored ADC file that is not valid JSON, or an unreadable gcloud configuration file). The finding carries the error message. |
| 2 | `CONFIG MISSING` | No gcloud configuration named `config_name` exists locally. |
| 3 | `AUTH REQUIRED` | The gcloud configuration has no `core/account`, or the credential check reported that account as not credentialed, or a gcloud operation in this session reported that re-authentication is required. |
| 4 | `ADC MISSING` | No stored ADC file exists for the account. |
| 5 | `PROJECT MISMATCH` | The gcloud configuration's `core/project` is set and differs from the account's `project_id`. |
| 6 | `READY` | None of the above apply. |

"Active" SHALL NOT be a health status. Whether an account is the gctx active account SHALL be reported as a separate boolean.

#### Scenario: Missing ADC
- **WHEN** an account's gcloud configuration exists with matching project and a credentialed account, and no stored ADC file exists
- **THEN** its primary status is `ADC MISSING`

#### Scenario: Multiple findings
- **WHEN** an account has no stored ADC and its gcloud configuration's project differs from `project_id`
- **THEN** its primary status is `ADC MISSING`
- **AND** its findings include both `ADC MISSING` and `PROJECT MISMATCH`

#### Scenario: Corrupt stored ADC
- **WHEN** an account's stored ADC file exists but is not valid JSON
- **THEN** its primary status is `ERROR` with a message stating the saved ADC file is invalid
- **AND** the message does not contain any bytes of the file

#### Scenario: Healthy account
- **WHEN** an account has a valid stored ADC, an existing gcloud configuration whose project equals `project_id`, and a credentialed `core/account`
- **THEN** its primary status is `READY`

### Requirement: Asynchronous credential check
The system SHALL provide a credential check that determines which gcloud accounts hold credentials. It does this by listing credentialed accounts through gcloud without network access. It SHALL NOT print, request, or refresh access tokens, and it SHALL NOT trigger any interactive authentication. Until the check completes, statuses SHALL be based on local inputs only. A failure of the check (gcloud missing, non-zero exit, timeout of 15 seconds) SHALL leave statuses unchanged and SHALL be reported as a single warning.

#### Scenario: Uncredentialed account
- **WHEN** the gcloud configuration for `sandbox` has `core/account = a@example.com` and the credential check does not list `a@example.com`
- **THEN** `sandbox` gains the `AUTH REQUIRED` finding

#### Scenario: gcloud not installed
- **WHEN** `gcloud` is not found on PATH and a credential check is requested
- **THEN** the check returns a `gcloud-not-found` warning
- **AND** all local statuses are still returned

### Requirement: Account name validation
Account names SHALL match `^[a-z][a-z0-9-]*$`, which is the gcloud configuration-name rule applied to `<name>-config`. Names SHALL be validated before any gcloud configuration, file, or config entry is created. Validation SHALL reject names containing path separators, `..`, whitespace, or uppercase letters.

#### Scenario: Invalid name rejected before side effects
- **WHEN** create is requested with name `Work`
- **THEN** it fails with an error stating names must start with a lowercase letter and contain only lowercase letters, digits, and hyphens
- **AND** no gcloud process is started and `config.json` is unchanged

#### Scenario: Path traversal rejected
- **WHEN** create is requested with name `../evil`
- **THEN** it fails validation and no file is created outside the gctx ADC directory

### Requirement: Create account
Create SHALL perform these steps in order:
1. validate the name
2. fail if an account with that name exists
3. create (or reuse) the gcloud configuration `<name>-config`
4. activate it
5. attempt to set its project
6. persist the account record
7. hand off to the login flow

A failure to set the project SHALL be a warning, not an error. If a gcloud configuration named `<name>-config` already exists and no gctx account references it, the operation SHALL report a `config-reused` warning. If login does not complete, the account record SHALL remain and its status SHALL reflect what is missing.

#### Scenario: Duplicate account checked first
- **WHEN** create is requested for an existing account name
- **THEN** it fails with `account '<name>' already exists`
- **AND** no gcloud configuration is created or activated

#### Scenario: Create succeeds but login is cancelled
- **WHEN** create completes steps 1–6 and the login flow is cancelled by the user
- **THEN** the account exists in `config.json`
- **AND** its status is `ADC MISSING` or `AUTH REQUIRED`

### Requirement: Login flow
Login SHALL:
1. verify the account exists
2. activate the account's gcloud configuration
3. run interactive `gcloud auth login`
4. run interactive `gcloud auth application-default login`
5. set the project again (failure is a warning)
6. verify the default ADC file exists and is valid JSON
7. copy it into the account's stored ADC location
8. record the stored path and the email from the account's own gcloud configuration

If step 3 or step 4 exits non-zero, the flow SHALL stop and report which step failed. No stored ADC SHALL be modified in that case. The ADC state SHALL be captured *before* step 4. The verification in step 6 SHALL NOT impose a fixed wait. If the file was not rewritten during step 4, the flow SHALL still save it and SHALL return an `adc-not-refreshed` warning.

#### Scenario: Login cancelled during browser auth
- **WHEN** `gcloud auth login` exits non-zero (for example after Ctrl+C)
- **THEN** login returns an error identifying the `auth login` step
- **AND** the account's stored ADC file and `adc_path` are unchanged

#### Scenario: ADC step fails after gcloud login succeeds
- **WHEN** `gcloud auth login` succeeds and `gcloud auth application-default login` exits non-zero
- **THEN** login returns an error identifying the `application-default login` step
- **AND** the result states that gcloud CLI credentials were updated but ADC was not saved

#### Scenario: No fixed post-login delay
- **WHEN** both auth steps succeed and the default ADC file was rewritten during step 4
- **THEN** the save step begins without waiting for a timeout

### Requirement: Save credentials
Save SHALL copy the current default ADC file into the account's stored ADC location with owner-only permissions. It SHALL first verify that the account exists and that the default ADC file exists and is valid JSON. It SHALL record the email from the account's own gcloud configuration (`core/account`), not from the globally active configuration.

#### Scenario: No default ADC
- **WHEN** save is requested and the default ADC file does not exist
- **THEN** it fails with `no ADC found at <path>` and nothing is written

#### Scenario: Email from the account's configuration
- **WHEN** save is requested for `sandbox` while the globally active gcloud configuration is `work-config`
- **THEN** the recorded email is the `core/account` of `sandbox-config`

### Requirement: Switch account
Switch SHALL:
1. verify the account exists
2. restore its stored ADC to the default ADC location atomically (a missing stored ADC is an `adc-missing` warning)
3. activate its gcloud configuration (failure is an error)
4. set the configuration's project (failure is a warning; a re-authentication failure is the `reauth-required` warning)
5. set the gctx active account

A failure to persist the active account SHALL be returned as an error.

#### Scenario: Switch without saved ADC
- **WHEN** switch is requested for an account without stored ADC
- **THEN** the gcloud configuration is activated and the account becomes active
- **AND** the result contains an `adc-missing` warning

#### Scenario: Re-authentication required
- **WHEN** setting the project fails with output containing `Reauthentication required`
- **THEN** switch succeeds with a `reauth-required` warning
- **AND** the account gains the session-scoped `AUTH REQUIRED` finding

#### Scenario: Activation fails
- **WHEN** activating the gcloud configuration fails
- **THEN** switch returns an error and the gctx active account is unchanged

### Requirement: Delete account
Delete SHALL:
1. remove the account's stored ADC file(s)
2. remove the gcloud configuration only when explicitly requested
3. remove the account record, clearing the active account if it was this account

Each step's outcome SHALL be reported. The default ADC file SHALL never be deleted. ADC removal SHALL NOT be optional (see credential-safety). If removing the gcloud configuration fails (for example because it is gcloud's active configuration), the account record SHALL still be removed and a `config-delete-failed` warning returned.

#### Scenario: Delete keeps gcloud configuration by default
- **WHEN** delete is requested without the gcloud-configuration option
- **THEN** the gcloud configuration still exists and the account is gone from `config.json`

#### Scenario: gcloud configuration deletion fails
- **WHEN** delete is requested with the gcloud-configuration option and gcloud refuses to delete it
- **THEN** the account is removed from `config.json`
- **AND** the result contains a `config-delete-failed` warning with gcloud's error message

### Requirement: Isolated execution environment
Run and shell SHALL execute their child process with the parent environment plus:
- `CLOUDSDK_ACTIVE_CONFIG_NAME=<config_name>`
- `GOOGLE_APPLICATION_CREDENTIALS=<stored ADC path, or empty when none>`

Shell SHALL additionally set `GCTX_ACCOUNT=<name>`. These operations SHALL NOT modify `config.json`, the default ADC file, or the globally active gcloud configuration. Run SHALL pass user arguments to `gcloud` as an argument vector without invoking a shell.

#### Scenario: Run does not change global state
- **WHEN** run is executed for `personal` while `work` is active
- **THEN** the child sees `CLOUDSDK_ACTIVE_CONFIG_NAME=personal-config`
- **AND** afterwards the gctx active account is `work` and the default ADC file is byte-identical to before

#### Scenario: Shell environment
- **WHEN** a shell is started for `personal`
- **THEN** the child environment contains `GCTX_ACCOUNT=personal`, `CLOUDSDK_ACTIVE_CONFIG_NAME=personal-config`, and `GOOGLE_APPLICATION_CREDENTIALS` set to the stored ADC path

### Requirement: Shell resolution
The shell program SHALL be `$SHELL` when it is set and non-empty. Otherwise it is, on Windows, the first available of `pwsh.exe`, `powershell.exe`, `%COMSPEC%`, `cmd.exe`, and on other platforms `/bin/sh`. On Windows, if `$SHELL` is set but cannot be resolved to an executable (for example, Git Bash's `/usr/bin/bash`), the Windows chain SHALL be used instead.

#### Scenario: Unresolvable SHELL on Windows
- **WHEN** running on Windows with `SHELL=/usr/bin/bash`, which is not resolvable, and `pwsh.exe` on PATH
- **THEN** the resolved shell is the path of `pwsh.exe`

#### Scenario: Windows without SHELL
- **WHEN** running on Windows with `SHELL` unset, `pwsh.exe` absent, and `powershell.exe` on PATH
- **THEN** the resolved shell is the path of `powershell.exe`

#### Scenario: Unix respects SHELL
- **WHEN** `SHELL=/usr/bin/fish`
- **THEN** the resolved shell is `/usr/bin/fish`

### Requirement: Account-scoped gcloud invocations target the account's configuration
Every gcloud invocation made on behalf of a specific account SHALL set `CLOUDSDK_ACTIVE_CONFIG_NAME=<config_name>` explicitly. This covers setting the project, `auth login`, `auth application-default login`, and reading `core/account`. The invocation then affects that account's configuration even when the parent environment already pins a different configuration (for example, inside `gctx shell`). The default ADC location SHALL be `<gcloud configuration directory>/application_default_credentials.json`. This is identical to the previous location whenever `CLOUDSDK_CONFIG` is unset.

#### Scenario: Switch from inside an isolated shell
- **WHEN** `gctx switch work` runs in an environment with `CLOUDSDK_ACTIVE_CONFIG_NAME=personal-config`
- **THEN** the project is set on `work-config`, and `personal-config`'s project is unchanged

#### Scenario: Custom gcloud directory
- **WHEN** `CLOUDSDK_CONFIG=/tmp/gc` and `gctx save work` runs
- **THEN** the ADC is read from `/tmp/gc/application_default_credentials.json`

### Requirement: Non-interactive gcloud invocations
Every gcloud invocation that is not explicitly interactive (`auth login`, `auth application-default login`, user `run`, shell) SHALL run with stdin connected to the null device and `CLOUDSDK_CORE_DISABLE_PROMPTS=1`, under a timeout. The timeout is 60 seconds for `config set project` and 30 seconds for other calls. A timeout SHALL be reported as an error naming the gcloud command.

#### Scenario: Hung gcloud
- **WHEN** `gcloud config configurations activate` does not exit within 30 seconds
- **THEN** the process is killed and the operation returns a timeout error naming that command

### Requirement: Safe persistence of account metadata
Writes to `config.json` SHALL be atomic: write to a temporary file in the same directory, then rename it over the target. Each mutating operation SHALL re-read `config.json` immediately before applying its change, so changes made by another gctx process since the last read are not discarded. The JSON schema of `config.json` SHALL NOT change.

#### Scenario: Concurrent CLI change is preserved
- **WHEN** a long-running TUI session loaded accounts {work, sandbox}, another process then adds `demo`, and the TUI switches to `sandbox`
- **THEN** `config.json` contains `work`, `sandbox`, and `demo`, with `active_account = sandbox`

#### Scenario: Interrupted write
- **WHEN** the process is killed while saving `config.json`
- **THEN** `config.json` contains either the previous or the new complete document, never a partial one
