# gctx: multi-account context manager

[![Documentation Status](https://readthedocs.org/projects/gctx/badge/?version=latest)](https://gctx.readthedocs.io/en/latest/?badge=latest)
[![CI](https://github.com/k0wl0n/gctx/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/k0wl0n/gctx/actions/workflows/ci.yml)

<img width="495" height="425" alt="image" src="https://github.com/user-attachments/assets/1a5d2c96-ab55-4a32-ae6d-dbdd1bf1434d" />


A Go CLI and terminal UI to keep several cloud accounts side by side and switch between them: GCP, AWS and Azure. GCP behaviour is unchanged (gcloud configuration plus ADC swap).


![01091-1-ezgif com-video-to-gif-converter](https://github.com/user-attachments/assets/f7f23e3f-e33a-49c1-bad4-3db753e1cfc8)


## Documentation

Full documentation is available at [gctx.readthedocs.io](https://gctx.readthedocs.io/en/latest/).

## Project Overview

**Problem**: Managing multiple GCP accounts requires constant re-authentication when switching between accounts, especially for ADC credentials which are stored in a single file.

**Solution**: `gctx` saves separate ADC credentials for each account and swaps them automatically when switching, eliminating the need for repeated `gcloud auth application-default login`.

## Providers

| Provider | An account is | How it switches | CLI |
|---|---|---|---|
| GCP | a gcloud configuration + stored ADC | global: `gctx switch` activates the gcloud configuration and swaps the ADC file | `gcloud` |
| AWS | an AWS profile | env: `AWS_PROFILE` | `aws` v2 |
| Azure | a subscription with its own isolated CLI directory | env: `AZURE_CONFIG_DIR` | `az` |

AWS and Azure have no global state to swap. `gctx switch` records the account as active for its slot (one active account per provider) and prints the line to apply it:

```bash
eval "$(gctx env prod-aws)"   # exports AWS_PROFILE for this shell
gctx shell prod-aws           # or start a subshell with the account applied
```

Details: [AWS](aws.md), [Azure](azure.md), [providers overview](providers.md).

## Commands

| Command | Example |
|---|---|
| `list` | `gctx list --provider aws` |
| `create` | `gctx create prod prod-profile --provider aws` |
| `env` | `eval "$(gctx env prod)"` |
| `switch` | `gctx switch work` |
| `login` | `gctx login work` |
| `delete` | `gctx delete old --gcloud-config` |
| `info` | `gctx info work` |
| `save` | `gctx save work` (GCP: store the current ADC) |
| `run` | `gctx run work compute instances list` (GCP) |
| `shell` | `gctx shell work` |
| `active` | `gctx active` |
| `tui` | `gctx tui` |

## Health states

| Status | Meaning |
|---|---|
| `READY` | Credentials are valid. |
| `AUTH REQUIRED` | Not logged in, or the provider reports re-authentication is needed. Run `gctx login <name>` (`l` in the TUI). |
| `EXPIRED` | The token or session is past its expiry. Log in again. |
| `UNVERIFIED` | The check timed out or the network failed. The last known result and its age are shown; it never turns into `AUTH REQUIRED`. |
| `CLI MISSING` | The provider's CLI is not on `PATH`. |
| `CONFIG MISSING` | GCP: no gcloud configuration named after the account. |
| `ADC MISSING` | GCP: no stored ADC file. |
| `PROJECT MISMATCH` | GCP: the configuration's project differs from the account's. |
| `ERROR` | A local check failed; details show the message. |

## Local state

- Config: `~/.config/gctx/config.json`. Account fields `provider` and `settings` and the top-level `active_by_slot` are new and optional; files written by earlier versions load unchanged and accounts without `provider` are GCP.
- Status cache: `status.json` in the user cache directory (`~/.cache/gctx` on Linux, `~/Library/Caches/gctx` on macOS, `%LocalAppData%\gctx` on Windows). It holds the last check per account: health, identity, expiry and check time. It never holds tokens, keys or secrets, and exists so the TUI's first paint is not blank.

| Variable | Effect |
|---|---|
| `GCTX_NO_TUI` | Non-empty: bare `gctx` prints help instead of opening the TUI. |
| `GCTX_ASCII` | `1` replaces Unicode glyphs with ASCII. |
| `GCTX_DEBUG` | `1` writes a redacted `tui-debug.log` (same as `gctx tui --debug`). |
| `NO_COLOR` | Disables colour; badges are shown as `[GCP]`. |

## Planned (not available yet)

None of the following exists in this version:

- AI tool accounts: Claude Code and Codex.
- AI gateway accounts: OpenRouter, SumoPod and OpenAI-compatible endpoints, with API keys read from references (`op://`, `gcpsm://`, `keychain:`, `env:`) instead of being stored.
- Tencent Cloud. It is not supported.
- A rename of the binary.

## Installation

### Homebrew (macOS/Linux)
```bash
brew tap k0wl0n/tap
brew install gctx
```

### Scoop (Windows)
```bash
scoop bucket add k0wl0n https://github.com/k0wl0n/scoop-bucket
scoop install gctx
```

### Manual
Download the latest binary from the [Releases](https://github.com/k0wl0n/gctx/releases) page and add it to your PATH.

### From Source
Requires Go 1.26 or newer.
```bash
go install github.com/k0wl0n/gctx@latest
```

## Development

### Prerequisites

This project uses [mise](https://mise.jdx.dev/) to manage dependencies (Go, Task, gcloud, GoReleaser).

```bash
# Install mise (if not installed)
curl https://mise.run | sh

# Install dependencies
mise install
```

This project uses [Task](https://taskfile.dev) (Taskfile) for build and management commands.

### Build & Install
```bash
# Build binary
task build

# Install to /usr/local/bin (requires sudo)
task install

# Run tests
task test

# Clean artifacts
task clean
```

### Configuration Management
```bash
# Show current config
task config:show

# Reset all configuration (DANGER)
task config:clean
```

### Demo / Testing Flow
```bash
# Create a demo account
task demo:create

# Switch to demo account
task demo:switch

# Re-authenticate demo account
task demo:login

# Delete demo account
task demo:delete
```

## Usage

### Interactive TUI

Run `gctx` with no arguments in a terminal to open a full-screen account manager. It lists every account with its health status, and lets you switch, log in, open an isolated shell, run a gcloud command, create, delete and save without remembering command names.

<!-- TODO: add TUI screenshot -->

`gctx` with no arguments opens the TUI only when all of these hold:

- stdin and stdout are terminals
- `TERM` is not `dumb`
- `CI` is unset, empty, `false` or `0`
- `GCTX_NO_TUI` is unset or empty

Otherwise it prints the usual help. Any argument (including `--help`) skips the TUI.

```bash
gctx            # open the TUI (when the conditions above hold)
gctx tui        # always open the TUI in a terminal, ignoring CI and GCTX_NO_TUI
GCTX_NO_TUI=1 gctx   # print help instead
gctx tui --debug     # write a debug log (same as GCTX_DEBUG=1)
```

| Key | Action |
|---|---|
| `j` / `↓`, `k` / `↑` | next / previous row |
| `g` / `home`, `G` / `end` | first / last row |
| `ctrl+f` / `pgdown`, `ctrl+b` / `pgup` | page down / up |
| `tab` / `shift+tab`, `1`-`9` | next / previous provider tab, jump to a tab |
| `enter` | switch to the selected account |
| `/` | filter (`esc` clears) |
| `d` / `i` | details |
| `l` | log in (Azure on a full-screen login page with the device code, `o` open browser, `y` copy code, `v` full log, `esc` cancel; other providers suspend the TUI) |
| `L` | log in every account that is AUTH REQUIRED or EXPIRED, one after the other (`n` skips to the next) |
| `s` | subshell with every active slot's account |
| `S` | subshell with only the selected account |
| `y` | copy the account's `eval "$(gctx env <name>)"` line |
| `r` | run the row's provider CLI (gcloud, aws or az) as the selected account |
| `c` | create an account: pick provider, name and the provider's fields; shows what is written and the CLI install hint when it is missing |
| `e` | edit the selected account: rename and change editable settings (identity fields are read-only). A rename moves the stored ADC file or isolated az directory with the account; names whose gcloud configuration, stored ADC or az directory belongs to another account are refused |
| `ctrl+d` | delete: lists what is removed (isolated Azure dir, optional provider logout first, gcloud configuration) and what is kept, asks you to type the account name, defaults to Cancel; refused while other accounts link to it |
| `a` | save the current ADC to the selected account |
| `p` | toggle the detail pane (150+ columns) |
| `z` | fold or unfold a provider group |
| `!` | only rows needing action |
| `o` | cycle sort: name, status, expiry |
| `R` | reload: re-locates provider CLIs and re-runs the auth checks (on a tab whose CLI is missing it is the rescan key) |
| `:` | command palette: fuzzy search over every action and account; the right column is the CLI equivalent, `enter` runs, `tab` completes |
| `?` | help: every key (generated from the key map) and a providers block with each CLI's version or install hint, plus the config and debug log paths; `j`/`k` scroll on a short terminal, `esc` or `?` closes |
| `q` / `ctrl+c` | quit |

Special states:

- **First run.** With no accounts, the TUI offers to import the existing gcloud configurations, `~/.aws/config` profiles and `~/.azure` subscriptions it finds (names, targets and non-secret settings only; discovery reads files and never runs a CLI or reads a token). Choose `Import N`, `Review each` or `Start empty`. Imported accounts show `AUTH REQUIRED` or `ADC MISSING` until you log in.
- **Missing CLI.** A provider whose CLI is not installed keeps a dim `<Label> n/a` tab. Selecting it explains what the CLI is needed for and shows the install command: `y` copies it, `R` rescans PATH.
- **Offline.** When every network check of a cycle fails, a banner says `network unreachable`, rows keep their last known status as `UNVERIFIED` with its age, and switching still works. The banner clears as soon as one check succeeds.

See the [TUI guide](https://gctx.readthedocs.io/en/latest/tui/) for health statuses and troubleshooting.

#### Windows notes

The TUI works in Windows Terminal and in conhost (PowerShell, Windows PowerShell, cmd.exe). Git Bash running in MinTTY does not provide a real terminal to native programs: bare `gctx` prints help and `gctx tui` fails with a hint. Run `winpty gctx tui`, or use Windows Terminal.

### Initial Setup
```bash
# Account 1 - automatically authenticates and saves credentials
gctx create work my-work-project
# Opens browser for auth + ADC, auto-saves

# Account 2
gctx create personal my-personal-project

# Account 3
gctx create client client-project
```

Pick the cloud with `--provider gcp|aws|azure` (default `gcp`). Tencent Cloud is not supported. Account names are unique across providers.

> **Note**: As of v0.5.0, `gctx create` always auto-authenticates. The `--auto-save` flag has been removed for simplicity.

### Daily Usage
```bash
# Switch accounts (instant, no re-auth!)
gctx switch work
gctx switch personal

# Re-authenticate an existing account
gctx login work
# This will run gcloud auth login and update saved ADC credentials

# Check current account
gctx active
# Output: Active account: work

# List all accounts
gctx list
# Output:
#   work (my-work-project) [user@work.com] ← active
#   personal (my-personal-project) [user@gmail.com]
#   client (client-project) [user@client.com]

# Show only one provider's accounts (a PROVIDER column appears whenever
# accounts of more than one provider exist, or --provider is given)
gctx list --provider gcp

# Print shell exports for an account (single-quoted, POSIX)
eval "$(gctx env personal)"

# Run command with specific account (Isolated!)
# This runs the command without changing your global active account
gctx run personal compute instances list

# Start an isolated shell session
# All gcloud commands in this shell will use 'personal' account
gctx shell personal
# (Type 'exit' to return to normal)

# Show account details
gctx info work

# Delete account
gctx delete old-account --gcloud-config
```

### AWS

Prerequisite: the `aws` CLI v2 on `PATH`. A missing CLI marks only AWS accounts `CLI MISSING`.

An AWS account is an AWS profile (`gctx create prod prod-profile --provider aws`). The profile can be adopted or created:

- **Adopt** (default, `auth=profile`): the profile must already exist in `~/.aws/config` (or `$AWS_CONFIG_FILE`), otherwise `create` fails with a clear error.
- **SSO** (`auth=sso`): the provider writes the profile with `aws configure set` from `sso_start_url`, `sso_region` and `region`, plus `sso_account_id` and `sso_role_name` if you supply them. Otherwise add those two once (`aws configure set profile.<name>.sso_account_id <id>`, `... .sso_role_name <role>`); credential checks fail without them, and `gctx login` prints the exact commands when they are missing. gctx never writes tokens or secrets, and never edits your AWS config on delete.

AWS has no global switch, so `gctx switch` prints a hint instead of changing state. Select the profile per shell:

```bash
eval "$(gctx env prod)"   # exports AWS_PROFILE=prod-profile
gctx login prod           # aws sso login --profile=prod-profile (SSO profiles only)
```

Credential checks use `aws sts get-caller-identity --profile=<profile>`. Profile names must start with a letter, digit or underscore, so a name can never be read as a flag.

The delete dialog offers `aws sso logout` (off by default). It signs out of every AWS SSO session on this machine, not only this profile: other AWS accounts need `gctx login` again afterwards.

### Azure

Prerequisite: the `az` CLI on `PATH`. A missing CLI marks only Azure accounts `CLI MISSING`.

An Azure account is a subscription id (`gctx create work 00000000-0000-0000-0000-000000000000 --provider azure`). The target must be a GUID. An optional `tenant` setting is passed to `az login --tenant`.

Each account gets its own private Azure CLI config directory, `~/.config/gctx/azure/<name>` (mode `0700`), used as `AZURE_CONFIG_DIR`. Accounts never share a login and nothing in `~/.azure` is touched. Deleting an account removes its directory.

Azure has no global switch, so `gctx switch` prints a hint instead of changing state. Select the account per shell:

```bash
eval "$(gctx env work)"   # exports AZURE_CONFIG_DIR=~/.config/gctx/azure/work
gctx login work           # az login --use-device-code, then az account set --subscription <id>
```

Login uses the device-code flow: open the printed URL and enter the code. Credential checks run `az account get-access-token --query expiresOn`, so gctx only sees the expiry time. gctx never reads token caches and never receives an access token.

### Isolated Sessions

`gctx` provides two ways to use an account without switching your global configuration:

1.  **Single Command (`gctx run`)**:
    Runs a single `gcloud` command with the specified account.
    ```bash
    gctx run my-account storage buckets list
    ```

2.  **Shell Session (`gctx shell`)**:
    Starts a new shell where the specified account is active. This does **not** affect other terminals or your global configuration.
    
    **Cross-platform support**: Automatically detects and uses the appropriate shell:
    - **Windows**: PowerShell Core → Windows PowerShell → cmd.exe
    - **Unix/Linux/Mac**: Uses `$SHELL` environment variable or `/bin/sh`
    
    ```bash
    gctx shell my-account
    
    # Now you are in an isolated session
    gcloud auth list  # shows my-account
    
    exit # Return to previous context
    ```

## Shell Completion

### Bash
```bash
gctx completion bash > /usr/local/etc/bash_completion.d/gctx
```

### Zsh
```bash
gctx completion zsh > "${fpath[1]}/_gctx"
```

### Fish
```bash
gctx completion fish > ~/.config/fish/completions/gctx.fish
```

## Prompt Integration
Add to `.bashrc` or `.zshrc`:
```bash
# Show active gctx account in prompt
gctx_prompt() {
    local active=$(gctx active 2>/dev/null | cut -d: -f2 | tr -d ' ')
    if [ -n "$active" ]; then
        echo "($active) "
    fi
}

PS1='$(gctx_prompt)$ '
```

## Updating gctx

### Homebrew (macOS/Linux)
```bash
brew update
brew upgrade gctx
```

### Scoop (Windows)
```bash
scoop update
scoop update gctx
```

### Manual Update
1. Download the latest binary from [Releases](https://github.com/k0wl0n/gctx/releases)
2. Replace your existing `gctx` binary
3. Verify the update:
   ```bash
   gctx version
   ```

### From Source
```bash
go install github.com/k0wl0n/gctx@latest
```

## Upcoming Release

### Behavior change
- **Bare `gctx` opens the interactive TUI** when run in a terminal. Previously it printed help. Scripts, pipes, `CI` environments and `GCTX_NO_TUI=1` still get the help text, and any argument behaves as before.

### Features
- `gctx tui` command and full-screen account manager with health statuses, filtering, details, switch, login, shell, run, create, delete and save.
- Accounts are checked locally for missing ADC, missing gcloud configuration, required re-authentication and project mismatch.
- The last credential check of each account (health, identity, expiry, budget and check time; never a token or key) is cached in `status.json` under the user cache directory (`gctx/`, mode `0600`), so status shows at once on the next launch. A check that times out keeps the last good result instead of reporting AUTH REQUIRED.

### Requirements
- Building from source requires Go 1.26 or newer.
