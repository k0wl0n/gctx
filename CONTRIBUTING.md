# Contributing to gctx

First off, thanks for taking the time to contribute! 🎉

## How to Contribute

### Reporting Bugs

1. Check the [issue tracker](https://github.com/k0wl0n/gctx/issues) to see if the bug has already been reported.
2. If not, open a new issue using the **Bug Report** template.
3. Provide as much detail as possible, including steps to reproduce and environment details.

### Suggesting Enhancements

1. Check the [issue tracker](https://github.com/k0wl0n/gctx/issues) to see if the feature has already been requested.
2. If not, open a new issue using the **Feature Request** template.
3. Explain why this feature would be useful and how it should work.

### Pull Requests

1. Fork the repository.
2. Create a new branch for your feature or fix (`git checkout -b feature/amazing-feature`).
3. Make your changes.
4. Run tests to ensure everything is working (`task test`).
5. Commit your changes (`git commit -m 'feat: add amazing feature'`).
6. Push to the branch (`git push origin feature/amazing-feature`).
7. Open a Pull Request.

## Development Setup

See the [Development](https://gctx.readthedocs.io/en/latest/#development) section in the documentation for setup instructions.

## Coding Style

- Follow standard Go conventions (run `go fmt ./...`).
- Write tests for new functionality.
- Ensure all tests pass before submitting.

## Testing

```bash
go test ./...          # everything
go test -short ./...   # skips the slow cases (PTY and latency tests)
go test ./cmd -run <TestName> -update   # rewrite golden files in cmd/testdata
```

Tests never call the real `gcloud`. `internal/testutil/fakegcloud` is a fake gcloud binary, and `internal/testutil/harness.go` builds an isolated HOME/APPDATA/`CLOUDSDK_CONFIG` with the fake first on `PATH`. Failures and latency can be injected with `FAKE_GCLOUD_FAIL="verb:exit[:stderr];..."` and `FAKE_GCLOUD_SLEEP="verb:ms"`.

Only regenerate goldens (`-update`) for an intended output change, and review the diff. CLI output is otherwise kept stable.

Never print, log or render ADC contents or tokens, and keep tests asserting that.

Before committing, run `go build ./... && go vet ./... && go test -short ./...`, and keep `go mod tidy -diff` empty. Dependencies are pinned to exact versions at least 7 days old.

## Manual verification for TUI changes

Automated tests cannot cover real terminals or browser auth. For changes to `pkg/tui` or terminal handling, check the following on macOS (Terminal.app or iTerm2), Linux (plus a tmux session) and Windows 11 (Windows Terminal with PowerShell 7 and Windows PowerShell 5.1, conhost with cmd.exe, Git Bash/MinTTY):

1. Bare `gctx` opens the TUI. `echo | gctx`, `gctx > f.txt`, `CI=1 gctx` and `GCTX_NO_TUI=1 gctx` print help.
2. Navigation, filter, details, help and `q` work. After quitting the prompt is on the normal screen, scrollback is intact and echo is on.
3. `l` with real browser auth: the TUI suspends, both flows complete, the TUI resumes with ADC shown as present.
4. `l` then `ctrl+c` at the browser prompt: Enter prompt, cancel message, terminal sane.
5. `r` with `projects list --limit=1`: readable output, exit line, Enter returns. Also `ctrl+c` during a slow command.
6. `s` starts the correct shell, `gcloud config list` shows the account, and `exit` returns.
7. `c` full create with real auth, then `ctrl+d` delete in both variants.
8. Resize the window to tiny and back; split and zoom a tmux pane.
9. `NO_COLOR=1` and `GCTX_ASCII=1` render legibly.
10. Git Bash/MinTTY: `gctx` prints help, `gctx tui` prints the winpty hint, `winpty gctx tui` works.
11. `GCTX_DEBUG=1 gctx`: the log is created with mode 0600 and contains no ADC JSON, tokens or run arguments.
