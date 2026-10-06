# Agent B4: documentation

Agent id: `b4-docs`. Read `agents/shared-context.md` first.
Work only inside the worktree path you were given, on its branch. You don't depend on any code; write from the specs and design.

## You own (edit only these)
- `README.md`
- `readthedocs/docs/tui.md` (new)
- `readthedocs/docs/architecture.md`
- `readthedocs/mkdocs.yml`
- `CONTRIBUTING.md`

## Tasks
- 10.1: README.
  - New "Interactive TUI" section near the top of Usage: launch rules, `gctx tui`, `GCTX_NO_TUI`, keybinding table, and a screenshot placeholder comment.
  - "Windows notes" subsection: Windows Terminal/conhost; MinTTY/Git Bash needs `winpty` or Windows Terminal.
  - Go ≥ 1.26 under "From Source".
  - Replace "What's New in v0.5.0" with an upcoming-release section that includes the bare-`gctx` behavior change.
- 10.2: `readthedocs/docs/tui.md`.
  - usage and keybindings
  - health statuses with meanings and remedies (from specs/account-management)
  - run-prompt tokenizer rules
  - troubleshooting: terminal too small, `TERM=dumb`, MinTTY, `NO_COLOR`/`GCTX_ASCII`, debug log location, restoring a broken terminal (`reset` / `stty sane` / reopen the tab)
  - `architecture.md`: the new package layout and Mermaid architecture diagram from design.md (no `watcher`)
  - `mkdocs.yml`: add the TUI page to nav, and `gctx_tui.md` under CLI Reference
- 10.3: `CONTRIBUTING.md`. How to run tests (fake gcloud, `-short` for the slow cases, `-update` for goldens) and the manual verification matrix for TUI changes.

Don't run `go run main.go docs`; the integration agent regenerates the CLI reference last. Keep the existing README structure and tone.

Commit per task. Tick 10.1–10.3 in tasks.md. Write `agents/reports/b4-docs.md` and commit it.
