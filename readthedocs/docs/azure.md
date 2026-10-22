# Azure

Prerequisite: the `az` CLI on `PATH`. A missing CLI marks only Azure accounts `CLI MISSING`.

An Azure account is a subscription id (`gctx create work 00000000-0000-0000-0000-000000000000 --provider azure`). The target must be a GUID. An optional `tenant` setting is passed to `az login --tenant`.

Each account gets its own private Azure CLI config directory, `~/.config/gctx/azure/<name>` (mode `0700`), used as `AZURE_CONFIG_DIR`. Accounts never share a login and nothing in `~/.azure` is touched. Deleting an account removes its directory.

Azure has no global switch, so `gctx switch` prints a hint instead of changing state. Select the account per shell:

```bash
eval "$(gctx env work)"   # exports AZURE_CONFIG_DIR=~/.config/gctx/azure/work
gctx login work           # az login --use-device-code, then az account set --subscription <id>
```

Login uses the device-code flow: open the printed URL and enter the code. Credential checks run `az account get-access-token --query expiresOn`, so gctx only sees the expiry time. gctx never reads token caches and never receives an access token.
