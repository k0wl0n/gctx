# GCloud Multi-Account Manager

[![Documentation Status](https://readthedocs.org/projects/gctx/badge/?version=latest)](https://gctx.readthedocs.io/en/latest/?badge=latest)
[![CI](https://github.com/k0wl0n/gctx/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/k0wl0n/gctx/actions/workflows/ci.yml)

<img width="495" height="425" alt="image" src="https://github.com/user-attachments/assets/1a5d2c96-ab55-4a32-ae6d-dbdd1bf1434d" />


A Go-based CLI tool using Cobra to manage multiple GCP accounts with seamless switching of both gcloud configurations and ADC (Application Default Credentials).


![01091-1-ezgif com-video-to-gif-converter](https://github.com/user-attachments/assets/f7f23e3f-e33a-49c1-bad4-3db753e1cfc8)


## Documentation

Full documentation is available at [gctx.readthedocs.io](https://gctx.readthedocs.io/en/latest/).

## Project Overview

**Problem**: Managing multiple GCP accounts requires constant re-authentication when switching between accounts, especially for ADC credentials which are stored in a single file.

**Solution**: `gctx` saves separate ADC credentials for each account and swaps them automatically when switching, eliminating the need for repeated `gcloud auth application-default login`.

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
| `enter` | switch to the selected account |
| `/` | filter (`esc` clears) |
| `d` / `i` | details |
| `l` | log in (suspends the TUI for the browser flows) |
| `s` | isolated shell |
| `r` | run a gcloud command as the selected account |
| `c` | create an account |
| `ctrl+d` | delete (confirmation defaults to Cancel) |
| `a` | save the current ADC to the selected account |
| `R` | reload |
| `?` | help |
| `q` / `ctrl+c` | quit |

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

Pick the cloud with `--provider gcp|aws|azure|tencent` (default `gcp`); only providers registered in your build are accepted. Account names are unique across providers.

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

### Requirements
- Building from source requires Go 1.26 or newer.
