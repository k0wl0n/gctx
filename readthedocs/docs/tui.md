# Interactive TUI

`gctx` includes a full-screen terminal interface for managing accounts.

## Starting the TUI

```bash
gctx          # opens the TUI when launch rules allow it
gctx tui      # always opens it in a terminal
gctx tui --debug
```

Bare `gctx` opens the TUI only when stdin and stdout are terminals, `TERM` is not `dumb`, `CI` is unset, empty, `false` or `0`, and `GCTX_NO_TUI` is unset or empty. Otherwise it prints help. `gctx tui` ignores `CI` and `GCTX_NO_TUI`, but still needs a terminal; otherwise it exits with `gctx tui requires an interactive terminal`.

The screen shows a header (version, active account, gcloud configuration, default ADC state), the account table, a details pane at widths of 120 columns or more, a status line and a key-hint line. It uses the alternate screen, so your scrollback is untouched on exit.

## Keybindings

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

Notes:

- `d` opens details, not delete. Delete is `ctrl+d`, and its dialog starts with focus on Cancel.
- Filtering is a case-insensitive substring match on name, project, email and gcloud configuration. While the filter input is open, letters are text, not commands.
- `esc` cancels any dialog. `ctrl+c` quits unless a command is running.
- `:` is reserved and currently shows `Command mode is not available yet`.
- Only one changing operation (switch, save, delete, create, login) runs at a time.

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
| `READY` | None of the above. | None. |

Health is computed from local files and `gcloud auth list`. No network call or token refresh is made, and credentials are never displayed.

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
