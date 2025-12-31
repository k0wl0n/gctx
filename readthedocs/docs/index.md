# GCloud Multi-Account Manager (Go + Cobra)

[![Documentation Status](https://readthedocs.org/projects/gctx/badge/?version=latest)](https://gctx.readthedocs.io/en/latest/?badge=latest)

A Go-based CLI tool using Cobra to manage multiple GCP accounts with seamless switching of both gcloud configurations and ADC (Application Default Credentials).

## Documentation

Full documentation is available at [gctx.readthedocs.io](https://gctx.readthedocs.io/en/latest/).

## Project Overview

**Source Code**: [https://github.com/k0wl0n/gctx](https://github.com/k0wl0n/gctx)

**Problem**: Managing multiple GCP accounts requires constant re-authentication when switching between accounts, especially for ADC credentials which are stored in a single file.

**Solution**: `gctx` saves separate ADC credentials for each account and swaps them automatically when switching, eliminating the need for repeated `gcloud auth application-default login`.

## Installation

```bash
# Using Task (Recommended)
task install

# Manual
go install
# or
go build -o gctx
sudo cp gctx /usr/local/bin/
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

### Initial Setup (Auto-save)
```bash
# Account 1
gctx create work my-work-project --auto-save
# Opens browser for auth + ADC, auto-saves

# Account 2
gctx create personal my-personal-project --auto-save

# Account 3
gctx create client client-project --auto-save
```

### Initial Setup (Manual)
```bash
gctx create work my-work-project
gcloud auth login
gcloud auth application-default login
gctx save work
```

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

# Run command with specific account
gctx run personal compute instances list

# Show account details
gctx info work

# Delete account
gctx delete old-account --gcloud-config
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
