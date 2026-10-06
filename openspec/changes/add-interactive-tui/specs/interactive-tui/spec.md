## Purpose

Defines the observable behavior of the k9s-inspired full-screen terminal interface for gctx: layout, navigation, keybindings, filtering, modals, async and subprocess-backed actions, refresh, resizing, empty and error states, and terminal restoration.

## ADDED Requirements

### Requirement: Screen layout
The main screen SHALL have these regions, top to bottom:
1. **Header line:** `gctx <version>`, the active account and its project (or `Active: none`), the globally active gcloud configuration, and the default-ADC indicator. When the gcloud configuration differs from the active account's configuration, the header also shows `⚠`. When `GCTX_ACCOUNT` is set, it shows `Shell: <name>`.
2. **Account table:** with a details pane at wide widths.
3. **Status line:** for messages, spinner, and the filter input.
4. **Key-hint line:** for the current context.

The default-ADC indicator SHALL be one of:
- `ADC: ✓ active` (the default ADC file is byte-identical to the active account's stored ADC)
- `ADC: ⚠ differs` (both files exist but differ)
- `ADC: ✗ none` (no default ADC file)
- `ADC: ? unknown` (no active account or no stored ADC to compare)

The TUI SHALL use the alternate screen and SHALL NOT enable mouse capture.

#### Scenario: Header reflects drift
- **WHEN** `work` is the gctx active account and gcloud's active configuration is `sandbox-config`
- **THEN** the header shows `Active: work (<project>)` and `gcloud: sandbox-config ⚠`

#### Scenario: Inside an isolated shell
- **WHEN** the TUI is started with `GCTX_ACCOUNT=personal`
- **THEN** the header shows `Shell: personal` in addition to the global active account

### Requirement: Account table
The table SHALL list every account sorted by name, one row each. The columns are:
- an active marker (`●` on the gctx active account, blank otherwise)
- `NAME`, `PROJECT`, `EMAIL` (`-` when unknown)
- `ADC` (`✓` or `✗`)
- `STATUS` (primary health status)

Status SHALL be conveyed by text, with color as reinforcement only. When `NO_COLOR` is set, no color SHALL be emitted. Text longer than its column SHALL be truncated with `…`. When `GCTX_ASCII=1`, the glyphs `● ✓ ✗ ⚠ …` SHALL be replaced by `* + x ! ...`.

#### Scenario: ASCII mode
- **WHEN** the TUI runs with `GCTX_ASCII=1`
- **THEN** the active row shows `*` and no non-ASCII glyph appears in the rendered view (box borders excepted)

#### Scenario: Active account highlighted
- **WHEN** the TUI opens with accounts `personal`, `sandbox`, `work` and `work` active
- **THEN** rows appear in the order `personal`, `sandbox`, `work`
- **AND** the `work` row shows `●`
- **AND** the selection starts on `work`

#### Scenario: No active account
- **WHEN** no account is active
- **THEN** the selection starts on the first row and the header shows `Active: none`

### Requirement: Responsive layout
The layout SHALL adapt to the terminal size:

| Width | Visible columns | Details pane |
|---|---|---|
| ≥ 120 | all columns | right-side details pane (≥ 40 columns) |
| 90–119 | marker, `NAME`, `PROJECT`, `EMAIL`, `STATUS` | none |
| 60–89 | marker, `NAME`, `PROJECT`, `STATUS` | none; key hints shortened to `enter switch · ? help · q quit` |

Below 60 columns or 12 rows, the TUI SHALL replace the screen with `Terminal too small: need at least 60×12, current <W>×<H>`. In that state, only `q` and `ctrl+c` act. Resizing SHALL NOT panic, lose the selection, or clear an applied filter. The table SHALL scroll so the selected row is always visible.

#### Scenario: Narrow terminal hides optional columns
- **WHEN** the terminal is 80×24
- **THEN** `EMAIL`, `ADC`, and the details pane are not rendered and `NAME`, `PROJECT`, `STATUS` are

#### Scenario: Shrink below minimum and back
- **WHEN** the terminal is resized from 120×40 to 40×10 and back to 120×40 while `sandbox` is selected
- **THEN** the too-small message is shown at 40×10
- **AND** at 120×40 the full layout returns with `sandbox` still selected

#### Scenario: Selection kept visible
- **WHEN** 40 accounts exist in a 24-row terminal and the user presses `G`
- **THEN** the last account is selected and visible

### Requirement: Keybindings
In the main view, the TUI SHALL implement exactly these bindings:

| Key | Action |
|---|---|
| `j` / `↓` | next row |
| `k` / `↑` | previous row |
| `g` / `home` | first row |
| `G` / `end` | last row |
| `ctrl+f` / `pgdown` | page down |
| `ctrl+b` / `pgup` | page up |
| `enter` | switch to selected account |
| `/` | filter |
| `esc` | clear applied filter |
| `d` / `i` | details |
| `l` | login |
| `s` | isolated shell |
| `r` | run gcloud command |
| `c` | create |
| `ctrl+d` | delete |
| `a` | save current ADC to selected account |
| `R` | reload |
| `?` | help |
| `q` / `ctrl+c` | quit |

`:` SHALL be reserved. Pressing it SHALL show `Command mode is not available yet`. Keys that need a selection SHALL show `No account selected` when the table is empty. Navigation at the first or last row SHALL NOT wrap.

#### Scenario: Navigation bounds
- **WHEN** the first row is selected and the user presses `k`
- **THEN** the first row remains selected

#### Scenario: Delete is not on plain d
- **WHEN** the user presses `d`
- **THEN** the details view opens and no delete confirmation appears

#### Scenario: Reserved colon
- **WHEN** the user presses `:`
- **THEN** the status line shows `Command mode is not available yet` and nothing else changes

### Requirement: Filtering
`/` SHALL open a filter input in the status line. As the user types, rows SHALL be narrowed to accounts whose name, project ID, email, or gcloud configuration name contains the query (case-insensitive substring). Filter input works as follows:
- While typing, letters are input (not commands). `↑`/`↓` move the selection.
- `enter` closes the input and keeps the filter applied. The table title shows `filter: <query> (<matches>/<total>)`.
- `esc` while typing clears the query and closes the input.
- `esc` in the main view clears an applied filter.

If the selected account is still visible after filtering, it SHALL stay selected. Otherwise, the first match is selected. With zero matches, the table body SHALL show `No accounts match "<query>"`.

#### Scenario: Filter by project
- **WHEN** the user types `/sandbox` and presses `enter`
- **THEN** only accounts whose name, project, email, or config contains `sandbox` are listed
- **AND** the title shows `filter: sandbox (1/3)` for one match out of three

#### Scenario: Typing j in the filter
- **WHEN** the filter input is open and the user types `j`
- **THEN** `j` is appended to the query and the selection does not move

#### Scenario: Clear filter
- **WHEN** a filter is applied and the user presses `esc` in the main view
- **THEN** all accounts are listed again and the previously selected account remains selected

### Requirement: Details view
`d` or `i` SHALL show, for the selected account:
- name, project ID, email
- gcloud configuration name, and whether it exists
- the configuration's `core/account` and `core/project`
- stored ADC path, existence, and modification time
- creation date, and whether it is active
- every health finding with a one-line remedy:
  - `ADC MISSING` → `press l to log in, or a to save the current ADC`
  - `AUTH REQUIRED` → `press l to log in`
  - `CONFIG MISSING` → `delete with ctrl+d and recreate with c`
  - `PROJECT MISMATCH` → `press enter to re-apply the project`
  - `ERROR` → the error message

At widths ≥ 120 the same content SHALL be shown continuously in the details pane. `esc`, `d`, `i`, or `q` SHALL close the full-screen details view.

#### Scenario: Details for an account without ADC
- **WHEN** the user opens details for `sandbox`, which has no stored ADC
- **THEN** the view lists `ADC MISSING — press l to log in, or a to save the current ADC`

### Requirement: Switch action
`enter` SHALL run the switch operation for the selected account in the background, showing a spinner and `Switching to <name>…`. On success, the TUI SHALL reload statuses, show `Switched to <name> (<project>)`, and mark the account active. Warnings SHALL be shown in the status line (`adc-missing` → `Switched to <name>, but no saved ADC — press l to log in`, `reauth-required` → `Switched to <name>, but gcloud requires re-authentication — press l`). An error SHALL leave the previously active account marked and show the error. Pressing `enter` on the already active account SHALL re-apply it.

#### Scenario: Switch to sandbox
- **WHEN** the user selects `sandbox` and presses `enter`
- **THEN** after the operation `sandbox` shows `●`
- **AND** the header shows `Active: sandbox` and `gcloud: sandbox-config`

#### Scenario: Switch with missing ADC
- **WHEN** the user switches to an account without stored ADC
- **THEN** the switch completes, the status line shows the `no saved ADC` warning, and the row's status is `ADC MISSING`

### Requirement: Operation concurrency
At most one mutating operation (switch, save, delete, create, login) SHALL be in flight at a time. The read-only credential check and local reloads MAY run alongside it. While a mutating operation is running:
- other mutating keys SHALL show `Busy: <operation description>`
- navigation, filtering, details, and help SHALL remain usable
- results of a superseded or stale background operation SHALL be ignored

`q` SHALL show `Busy: <operation> — wait for it to finish, or press ctrl+c twice to force quit`. A first `ctrl+c` SHALL show `Operation in progress — press ctrl+c again within 3s to quit`. A second `ctrl+c` within 3 seconds SHALL quit.

#### Scenario: Double action
- **WHEN** a switch is in progress and the user presses `ctrl+d`
- **THEN** no delete confirmation opens and the status line shows `Busy: switching to sandbox`

#### Scenario: Stale reload result
- **WHEN** a reload started before a delete returns after the delete's own reload
- **THEN** the table reflects the post-delete state

### Requirement: Create flow
`c` SHALL open a form with the fields `Account name` and `GCP project ID`. `tab`/`shift+tab`/`↑`/`↓` move between fields. `enter` on the first field moves to the second. `enter` on the second submits. `esc` cancels without side effects. On submit, the TUI SHALL validate locally and show errors under the offending field:

| Condition | Message |
|---|---|
| empty name | `Account name is required` |
| invalid name | `Use lowercase letters, digits and hyphens, starting with a letter` |
| duplicate name | `Account '<name>' already exists` |
| empty project | `Project ID is required` |
| invalid project (must match `^[a-z][a-z0-9-]{4,28}[a-z0-9]$` or the domain-scoped form `<domain>:<id>`) | `Project IDs are 6–30 characters: lowercase letters, digits, hyphens; start with a letter; no trailing hyphen` |

If `<name>-config` already exists locally and no gctx account references it, the form SHALL show `gcloud configuration <name>-config already exists and will be reused — press enter again to continue`. On valid submit, the TUI SHALL run the configuration steps in the background, then hand off to the login flow (suspended terminal). After return, it SHALL select the new account and show the outcome.

#### Scenario: Valid create
- **WHEN** the user enters `demo` and `demo-project` and submits
- **THEN** gcloud configuration `demo-config` is created and activated, the account is saved, and the terminal is handed to `gcloud auth login`

#### Scenario: Validation errors
- **WHEN** the user submits an empty name and project `ab`
- **THEN** both field errors are shown and no gcloud process is started

#### Scenario: Cancel create
- **WHEN** the user presses `esc` in the form
- **THEN** the form closes and `config.json` is unchanged

#### Scenario: Authentication not completed
- **WHEN** creation succeeds but login is cancelled
- **THEN** `demo` is listed and selected with status `ADC MISSING` or `AUTH REQUIRED`
- **AND** the status line shows `Account demo created, but authentication did not complete — press l to retry`

### Requirement: Delete flow
`ctrl+d` SHALL open a confirmation dialog containing:
- `Delete account "<name>"?`
- the fixed notice `Saved ADC credentials for this account will be removed.`
- a checkbox `Also delete gcloud configuration "<config>"`, unchecked by default
- `[Cancel]` and `[Delete]` buttons, with focus initially on `[Cancel]`

`tab`/`shift+tab`/`←`/`→` move focus. `space` toggles the checkbox when focused. `enter` activates the focused element. `esc` cancels. If the configuration is gcloud's currently active configuration, the checkbox SHALL be disabled with the note `cannot delete gcloud's active configuration`. After deletion, the TUI SHALL reload, select the nearest remaining row, and show `Deleted <name>`. Each warning SHALL be shown in the status line.

#### Scenario: Default delete keeps gcloud configuration
- **WHEN** the user presses `ctrl+d` on `sandbox`, moves focus to `[Delete]`, and presses `enter`
- **THEN** `sandbox` is removed and `sandbox-config` still exists in gcloud

#### Scenario: Delete including configuration
- **WHEN** the user toggles the checkbox on, focuses `[Delete]`, and presses `enter`
- **THEN** `sandbox` and `sandbox-config` are both removed

#### Scenario: Enter right after opening does not delete
- **WHEN** the user presses `ctrl+d` then immediately `enter`
- **THEN** the dialog closes and nothing is deleted

### Requirement: Save ADC flow
`a` SHALL open a confirmation dialog containing:
- `Save current ADC for "<name>"?`
- the default ADC path and its modification time
- the email configured for the account's gcloud configuration, when known
- `[Cancel]` (initial focus) and `[Save]`

If the default ADC file does not exist, the TUI SHALL show `No ADC found at <path>` instead of the dialog. On confirm, the TUI SHALL save in the background, reload, and show `Saved ADC for <name>`.

#### Scenario: Save confirmed
- **WHEN** the user confirms saving for `sandbox`
- **THEN** `sandbox`'s stored ADC equals the default ADC and its `ADC` column shows `✓`

### Requirement: Suspend and resume for interactive subprocesses
Login, run, and shell SHALL suspend the TUI before starting their subprocess. Suspending means:
- leave the alternate screen
- restore normal terminal mode
- give the subprocess the real stdin, stdout, and stderr

When the subprocess exits, the TUI SHALL resume in the same state (selection, filter), reload account statuses, and show the outcome. While a subprocess runs, the TUI SHALL NOT read input or draw. Interrupt signals (`ctrl+c`) SHALL be delivered to the subprocess, and the TUI SHALL ignore them. Browser authentication SHALL happen in the user's browser via gcloud and SHALL NOT be embedded in the TUI.

#### Scenario: Terminal restored after subprocess
- **WHEN** any suspended subprocess exits (successfully, with an error, or by signal)
- **THEN** the TUI redraws fully and keyboard input works without pressing extra keys

### Requirement: Login flow
`l` SHALL suspend the TUI and print `Logging in to <name> (<config>)…` to the terminal. It SHALL then run the account login flow with its normal progress messages. On success, it SHALL resume immediately and show `Login complete for <name>`. On failure or cancellation, it SHALL print the error and `Press Enter to return to gctx`, wait for Enter, resume, and show the failure in the status line:
- `Login cancelled for <name>; credentials unchanged`, when `gcloud auth login` failed
- `gcloud login succeeded but ADC was not saved for <name> — press l to retry`, when the ADC step failed

The selected account's status SHALL be refreshed and the credential check re-run.

#### Scenario: Successful login
- **WHEN** the user presses `l` on `sandbox` and completes both browser flows
- **THEN** the TUI resumes, `sandbox`'s `ADC` column shows `✓`, and the status line shows `Login complete for sandbox`

#### Scenario: Ctrl+C during browser login
- **WHEN** the user presses `ctrl+c` while `gcloud auth login` waits for the browser
- **THEN** gcloud exits, `Press Enter to return to gctx` is shown, and after Enter the TUI shows `Login cancelled for sandbox; credentials unchanged`

### Requirement: Run flow
`r` SHALL open a prompt titled `Run as: <name> (<config>)` with the input prefix `gcloud > `. The prompt behaves as follows:
- `↑`/`↓` recall up to 50 previous commands from the current session.
- `esc` cancels.
- Empty input SHALL NOT submit.
- Tokenizer and Windows-safety rejections SHALL be shown inline (see credential-safety).
- When the account has no stored ADC, the prompt SHALL show `⚠ No saved ADC: client libraries will not find credentials`.

On submit, the TUI SHALL:
1. suspend
2. print `── gctx: running as <name>: gcloud <args> ──`
3. run gcloud in the account's isolated environment
4. print `── exit status <N> · press Enter to return to gctx ──`
5. wait for Enter
6. resume, reload, and show `gcloud exited with status <N>` (error styling when N ≠ 0)

The gctx active account SHALL NOT change.

#### Scenario: Isolated run
- **WHEN** `work` is active and the user runs `config get-value project` as `personal`
- **THEN** the output shows `personal`'s project, the exit status line shows 0, and after Enter the header still shows `Active: work`

#### Scenario: Ctrl+C during run
- **WHEN** the user presses `ctrl+c` while the gcloud command runs
- **THEN** gcloud is interrupted, the exit status line is printed, and after Enter the TUI resumes

### Requirement: Shell flow
`s` SHALL suspend the TUI and print `Starting shell for account '<name>'... Type 'exit' to return to gctx.` It SHALL then start the resolved shell with the isolated environment. When the shell exits, the TUI SHALL resume immediately, reload, and show `Shell for <name> exited` (with ` (status <N>)` appended when N ≠ 0, as information, not as an error). `s` SHALL NOT require gcloud to be installed.

#### Scenario: Shell round trip
- **WHEN** the user presses `s` on `personal`, runs `echo $GCTX_ACCOUNT`, and types `exit`
- **THEN** the shell printed `personal` and the TUI is restored with the same selection

### Requirement: Credential check and refresh
After the first frame is drawn, the TUI SHALL start the asynchronous credential check without blocking input. While it runs, the status line SHALL show `Checking credentials…`. The TUI SHALL reload local statuses at these times:
- on `R` (which also re-runs the credential check)
- after every mutating operation
- after every suspended subprocess returns
- when the terminal reports regained focus (where supported)

The TUI SHALL NOT poll periodically. `R` SHALL show `Reloaded <N> accounts`.

#### Scenario: External change picked up
- **WHEN** another terminal runs `gctx create demo demo-project` and the user presses `R`
- **THEN** `demo` appears in the table

#### Scenario: Input stays responsive
- **WHEN** the credential check takes 3 seconds
- **THEN** navigation keys move the selection during that time

### Requirement: gcloud unavailable
If `gcloud` is not found on PATH at startup or reload:
- the header SHALL show `gcloud: not found`
- `enter`, `l`, `r`, `c`, and deleting with the configuration option SHALL show `gcloud not found on PATH — install the Google Cloud CLI`
- `s`, `a`, `d`, `/`, `?`, `R`, `q`, and delete without the configuration option SHALL keep working

#### Scenario: Switch without gcloud
- **WHEN** gcloud is not installed and the user presses `enter`
- **THEN** no subprocess is started and the status line shows the gcloud-not-found message

### Requirement: Empty and error states
With no accounts, the table area SHALL show `No accounts configured.` and `Press c to create your first account.` without table headers. If `config.json` cannot be read or parsed, the TUI SHALL show `Cannot read <path>: <error>`, `Fix the file and press R to retry, or q to quit.` In that state it SHALL disable every mutating action and SHALL NOT write the file.

#### Scenario: First run
- **WHEN** the TUI starts with no `config.json`
- **THEN** the empty-state message is shown and `c` opens the create form

#### Scenario: Corrupt config
- **WHEN** `config.json` contains invalid JSON
- **THEN** the error state is shown and the file's bytes are unchanged after the user quits

### Requirement: Status messages
Informational messages SHALL disappear after 5 seconds or on the next action, whichever is first. Warning and error messages SHALL persist until the next key press that triggers an action. Errors SHALL include the failing step and gcloud's last stderr line when available, truncated to the screen width.

#### Scenario: Error persists
- **WHEN** a switch fails with `failed to activate gcloud config`
- **THEN** the message remains visible after 10 seconds without input

### Requirement: Help screen
`?` SHALL open a full-screen help that lists every binding from the keybindings table, grouped into Navigation, Actions, and General. `?`, `esc`, or `q` SHALL close it.

#### Scenario: Help closes with q
- **WHEN** the help screen is open and the user presses `q`
- **THEN** the help closes and the TUI keeps running

### Requirement: Quit and terminal restoration
`q` or `ctrl+c` in the main view (with no operation in flight) SHALL exit with status 0. On every exit path, the terminal SHALL be restored to its prior mode, the main screen, and a visible cursor. Exit paths are normal quit, internal error, panic, and `SIGTERM`. `ctrl+c` inside the filter input or any dialog SHALL also quit.

#### Scenario: Clean quit
- **WHEN** the user presses `q`
- **THEN** the process exits 0 and the shell prompt appears on the normal screen with echo enabled

### Requirement: Startup performance
With up to 50 accounts on a local filesystem, the first frame SHALL be ready within 100 ms of TUI initialization. Initialization SHALL NOT start any gcloud subprocess or network request before the first frame is drawn.

#### Scenario: No subprocess before first frame
- **WHEN** the TUI starts with a fake gcloud that records invocations
- **THEN** no gcloud invocation is recorded before the first view is rendered
