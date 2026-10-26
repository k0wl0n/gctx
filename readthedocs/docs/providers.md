# Providers

`gctx` manages accounts of three providers. Each account belongs to one provider (`--provider` on `create`, default `gcp`). Account names are unique across providers. Tencent Cloud is not supported.

| Provider | Account | Switching | CLI |
|---|---|---|---|
| GCP | gcloud configuration + stored ADC | global: `gctx switch` activates the configuration and swaps the ADC file | `gcloud` |
| AWS | AWS profile | env: `AWS_PROFILE` | `aws` v2 |
| Azure | subscription with its own `AZURE_CONFIG_DIR` | env: `AZURE_CONFIG_DIR` | `az` |

## Slots

Each provider owns one slot and one account per slot is active (`active_by_slot` in the config). Switching an AWS account never changes the active GCP account. `gctx active` shows the active account of the GCP slot; the TUI header shows every slot.

## Env-based providers

AWS and Azure have no global state to swap. `gctx switch <name>` records the account as the slot's active account and prints how to apply it. Apply an account with:

```bash
eval "$(gctx env <name>)"   # exports the provider's variables, single-quoted
gctx shell <name>           # subshell with the account applied
```

In the TUI, `y` copies the `eval` line, `s` starts a subshell with every active slot applied and `S` one with only the selected account.

## Health

Credential checks run per provider, in parallel, each with its own timeout. Network failures and timeouts give `UNVERIFIED`, never `AUTH REQUIRED`. See [health states](tui.md#account-health). A missing CLI marks only that provider's accounts `CLI MISSING`.

See [AWS](aws.md) and [Azure](azure.md) for provider specifics.
