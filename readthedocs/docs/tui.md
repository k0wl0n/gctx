# Interactive TUI

`gctx` includes a full-screen terminal interface for managing accounts.

## Starting the TUI

```bash
gctx          # opens the TUI when launch rules allow it
gctx tui      # always opens it in a terminal
gctx tui --debug
```

Bare `gctx` opens the TUI only when stdin and stdout are terminals, `TERM` is not `dumb`, `CI` is unset, empty, `false` or `0`, and `GCTX_NO_TUI` is unset or empty. Otherwise it prints help. `gctx tui` ignores `CI` and `GCTX_NO_TUI`, but still needs a terminal; otherwise it exits with `gctx tui requires an interactive terminal`.

The screen shows, top to bottom:

- **Header**: one slot per provider that has accounts (`GCP ● cloudsuite  AWS ● prod-aws  AZR ○`, where `○` means no account of that provider is active), config health and the age of the last credential check. A dim second line shows the env the active accounts would export; key references appear as `‹op://…›`, never as values.
- **Tab row**: `All`, then one tab per provider with accounts. A provider whose CLI is missing shows `n/a`.
- **List**: on `All`, grouped by category (only clouds exist in this version; AI tools and gateways are planned) and provider, each group with a count summary. On a provider tab the KIND column drops and the provider's own columns (for example REGION for AWS) appear. As the terminal narrows, columns drop in this order: EXPIRES/BUDGET, IDENTITY, ADC, provider columns; TARGET then shortens with an ellipsis in the middle. NAME and STATUS always stay.
- **Detail pane** (150 columns or more, `p` toggles it): the selected account's details, the env activation applies and its actions. Narrower terminals use `d` for the full-screen details.
- **Status line**: credential check progress per provider and account totals.
- **Hint line**: the keys that fit the width.

Width tiers: 150 columns or more (wide, detail pane), 100-149 (standard), 60-99 (narrow). In the narrow tier the badge moves into the NAME column, the tab row becomes a single chip and the header slots become coloured dots. Colours follow the terminal's colour profile; with `NO_COLOR` badges are shown in brackets (`[GCP]`), and `GCTX_ASCII=1` uses ASCII glyphs. It uses the alternate screen, so your scrollback is untouched on exit.

## Keybindings

| Key | Action |
|---|---|
| `j` / `↓` | next row |
| `k` / `↑` | previous row |
| `g` / `home` | first row |
| `G` / `end` | last row |
| `ctrl+f` / `pgdown` | page down |
| `ctrl+b` / `pgup` | page up |
| `tab` / `shift+tab` | next / previous provider tab |
| `1`-`9` | jump to tab (`1` is All) |
| `enter` | switch to selected account |
| `/` | filter |
| `esc` | clear applied filter |
| `d` / `i` | details |
| `l` | login (Azure: full-screen login page; other providers suspend the TUI) |
| `L` | log in every AUTH REQUIRED or EXPIRED account in turn (`n` skips) |
| `s` | subshell with every active slot applied |
| `S` | subshell with only the selected account |
| `y` | copy the `eval "$(gctx env <name>)"` line (env-based providers) |
| `r` | run provider CLI command (gcloud, aws or az, by the row's provider) |
| `c` | create |
| `e` | edit: rename and change editable settings (identity fields are read-only) |
| `ctrl+d` | delete |
| `a` | save current ADC to selected account |
| `p` | toggle the detail pane |
| `z` | fold or unfold the selected provider group (shows only rows needing action plus a selectable `… N more ready` row; `z` on it expands the group) |
| `!` | only rows needing action |
| `o` | cycle sort: name, status, expiry |
| `R` | reload: re-locates provider CLIs and re-runs the auth checks (retry when offline, rescan on a tab whose CLI is missing) |
| `:` | command palette |
| `?` | help |
| `q` / `ctrl+c` | quit |

Notes:

- `d` opens details, not delete. Delete is `ctrl+d`, and its dialog starts with focus on Cancel.
- Filtering is a case-insensitive substring match on name, project, email and gcloud configuration, within the current tab. While the filter input is open, letters are text, not commands.
- `esc` cancels any dialog. `ctrl+c` quits unless a command is running.
- `:` opens the command palette; the right column shows the CLI equivalent of each entry.
- A missing provider CLI blocks switch, login and run only on that provider's rows. `a` applies only to accounts with ADC (GCP).
- Only one changing operation (switch, save, delete, create, login) runs at a time.

## Help

`?` lists every key in up to three columns, generated from the same key map that drives the keys and the hint line, then a providers block: for each provider its CLI and version (or `<cli> not found` with the install hint) and how it switches (`global switch`, or `env: <VARIABLES>`). The versions come from `<cli> --version`, run in the background when help first opens, so the screen appears at once and shows `…` until they arrive. The last line gives the config path and, with `--debug`, the log path. On a short terminal `j`/`k`, `ctrl+f`/`ctrl+b` and `g`/`G` scroll; `esc` or `?` closes.

## First run, missing CLI and offline

**First run.** With no accounts the TUI looks for existing setups and offers to import them: gcloud configurations (`<gcloud dir>/configurations/config_*`), AWS profiles (`~/.aws/config`, including SSO profiles) and Azure subscriptions (`~/.azure/azureProfile.json`). Discovery only reads those files: it starts no CLI, makes no network call and never reads tokens or credentials. Only names, targets (project, profile, subscription id), regions and SSO start URL and region are recorded; a gcloud configuration is reused under its existing name and nothing is created in gcloud. Tick sources and press `Import N`, press `Review each` to tick single items, or `Start empty`. Names are lower-cased and invalid characters become `-`; an item whose name is still invalid or already taken is skipped and reported. Imported accounts show `AUTH REQUIRED` or `ADC MISSING` until you log in with `l`. The offer is not shown if nothing importable is found, and not again in the same session once dismissed.

**Missing CLI.** A registered provider whose CLI is not on PATH keeps a dim `<Label> n/a` tab, even with no accounts; `tab` and `1`-`9` include it. With no accounts the tab explains what the CLI is needed for and shows the install command: `y` copies it, `R` rescans PATH. Other providers are unaffected.

**Offline.** When every network check of a cycle fails or times out, a `network unreachable · showing last known status` banner appears above the list. Rows keep their last known status as `UNVERIFIED` with its age, local checks (file reads) still run, and switching still works. `R` retries; the banner clears as soon as one check succeeds. One slow provider alone is not offline.

## Login, shell and run

`l`, `s` and `r` hand the terminal to a subprocess. The TUI leaves the alternate screen while it runs, and `ctrl+c` goes to the subprocess only. When it exits the TUI redraws with the same selection and filter.

- **Login** runs the normal `gcloud auth login` and ADC login flows in your browser. On failure it waits for Enter so you can read the error.
- **Shell** starts the isolated shell for the account (same as `gctx shell`).
- **Run** asks for a gcloud command, runs it as the selected account without changing the active account, then waits for Enter.

### Run prompt rules

The run prompt does not use a shell. Input is split into arguments by gctx and passed directly to `gcloud`:

- Whitespace separates arguments.
- `'...'` is literal.
- Inside `"..."`, a backslash escapes `"` and `\`.
- Outside quotes, a backslash escapes only whitespace, quotes and backslash; other backslashes are literal.
- A leading `gcloud` is dropped.
- An unterminated quote, or an unquoted `|`, `;`, `&`, `>`, `<`, `` ` `` or `$(`, is rejected. Use the shell (`s`) for pipes and redirects.
- On Windows, when `gcloud` is a `.cmd` or `.bat` file, the characters `& | < > ^ % !` are also rejected because `cmd.exe` would reinterpret them.

`↑` / `↓` recall up to 50 earlier commands from the current session.

## Account health

Each account shows one primary status (the first that applies in this order) and the details view lists every finding.

| Status | Meaning | Remedy |
|---|---|---|
| `ERROR` | A local check failed, for example an unreadable or invalid stored ADC file, or an unreadable gcloud configuration file. | Fix the reported problem; the message is shown in details. |
| `CONFIG MISSING` | No gcloud configuration named after the account exists locally. | Delete with `ctrl+d` and recreate with `c`. |
| `AUTH REQUIRED` | The configuration has no account, gcloud reports it is not credentialed, or a switch in this session reported that re-authentication is needed. | Press `l` to log in. |
| `ADC MISSING` | No stored ADC file for the account. | Press `l` to log in, or `a` to save the current ADC. |
| `PROJECT MISMATCH` | The configuration's project differs from the account's project. | Press `enter` to re-apply the project. |
| `EXPIRED` | AWS or Azure: the session is past its expiry. | Press `l`. |
| `UNVERIFIED` | A check timed out or the network failed. Shows the last known result and its age; never becomes `AUTH REQUIRED`. | Press `R` to recheck. |
| `CLI MISSING` | The provider's CLI is not on `PATH`. | Install it (the provider tab shows the command). |
| `READY` | None of the above. | None. |

GCP health is computed from local files and `gcloud auth list`; AWS and Azure run `aws sts get-caller-identity` and `az account get-access-token --query expiresOn`. Tokens are never read or displayed. The last result per account is cached in `status.json` (no secrets).

The header ADC indicator is `✓ active` (default ADC identical to the active account's stored ADC), `⚠ differs`, `✗ none` or `? unknown`. A `⚠` after the gcloud configuration means it differs from the active account's configuration. Inside `gctx shell`, the header also shows `Shell: <name>`.

## Troubleshooting

**Terminal too small.** Below 60 columns or 12 rows the TUI shows `Terminal too small: need at least 60×12`. Enlarge the window; only `q` and `ctrl+c` act meanwhile. Selection and filter are kept.

**`TERM=dumb`.** The TUI does not start; bare `gctx` prints help and `gctx tui` fails. Use a real terminal or set `TERM` to something like `xterm-256color`.

**Git Bash / MinTTY on Windows.** MinTTY gives native programs pipes instead of a terminal. Run `winpty gctx tui`, or use Windows Terminal.

**Colors and glyphs.** Set `NO_COLOR=1` to disable color; statuses are always shown as text. Set `GCTX_ASCII=1` to replace `● ✓ ✗ ⚠ …` with `* + x ! ...` on terminals without Unicode fonts.

**Debug log.** `GCTX_DEBUG=1` or `gctx tui --debug` writes events (operation, duration, exit code, account name) to `tui-debug.log` in your user cache directory:

- Linux: `~/.cache/gctx/tui-debug.log`
- macOS: `~/Library/Caches/gctx/tui-debug.log`
- Windows: `%LocalAppData%\gctx\tui-debug.log`

The file is created with mode `0600` and truncated at startup when larger than 1 MiB. It never contains credentials or command arguments.

**Terminal left in a bad state.** If the terminal shows stray characters or hides typed input after a crash, run `reset` (or `stty sane`), or close and reopen the terminal tab.
