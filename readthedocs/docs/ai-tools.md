# AI tools

`gctx` keeps several Claude Code and Codex logins side by side. Each account owns a private config directory the tool is pointed at through one environment variable, like Azure.

| Tool | Provider id | Env variable | Login | Credential check | CLI |
|---|---|---|---|---|---|
| Claude Code | `claude` | `CLAUDE_CONFIG_DIR` | `claude auth login` | `claude auth status` (exit 0 logged in, 1 logged out) | `claude` |
| Codex | `codex` | `CODEX_HOME` | `codex login` | `codex login status` (exit 0 / 1) | `codex` |

## Accounts

```bash
gctx create claude-work "Acme Org" --provider claude   # the second argument is a display label
gctx login claude-work                                   # runs claude auth login inside the account's dir
eval "$(gctx env claude-work)"                           # exports CLAUDE_CONFIG_DIR=~/.config/gctx/ai/claude-work
```

Each account gets `~/.config/gctx/ai/<name>` (mode `0700`). gctx never opens the files the tool keeps there (tokens, history, settings); health comes only from the tool's status command. The org or workspace label is free text and shown only.

AI tools are env-based: `gctx switch` records the account for the tool's slot (`claude` or `codex`) and prints the `eval` line. Sessions already running keep the account they started with; after `↵` the TUI says how many processes of that tool are running.

## Existing setups

On first run the TUI offers your existing `~/.claude` and `~/.codex` (or `$CLAUDE_CONFIG_DIR` / `$CODEX_HOME`) as accounts named `claude-default` and `codex-default` (with a numeric suffix if the name is taken). The account uses that directory in place: nothing is copied or moved, discovery only checks that the directory exists, and deleting the account never removes the directory. A directory already used by an account is not offered again.

An account on the tool's own default directory (`~/.claude`, `~/.codex`) leaves the variable unset: Claude Code keeps its login in the macOS keychain only while `CLAUDE_CONFIG_DIR` is unset, so exporting it, even with the same path, makes the login invisible. For such an account `gctx env` prints `unset CLAUDE_CONFIG_DIR`, and `gctx run`, `gctx shell` and the credential checks remove the variable from the tool's environment even when your shell exports it. Isolated accounts under `<config>/ai/<name>` still export it.

## Claude Code on a cloud account

A Claude Code account can run on a GCP or AWS account instead of a subscription login. In the create form pick Auth `via cloud account` and name the cloud account, backend and region:

| Backend | Cloud account | Env added to the linked account's env |
|---|---|---|
| `vertex` | GCP | `CLAUDE_CODE_USE_VERTEX=1`, `ANTHROPIC_VERTEX_PROJECT_ID=<project>`, `CLOUD_ML_REGION=<region>` |
| `bedrock` | AWS | `CLAUDE_CODE_USE_BEDROCK=1`, `AWS_REGION=<region>` |

These variable names come from the Claude Code documentation and have not been verified against a real Vertex or Bedrock account yet. Azure accounts cannot be linked. A linked account shows the health of the account it runs on, `l` logs in that account and `>` jumps to it. A cloud account that something runs on cannot be deleted until its dependants are removed or relinked.
