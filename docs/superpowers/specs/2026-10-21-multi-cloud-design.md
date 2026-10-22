# Multi-Cloud Context Manager: Design

Status: draft for review. Decisions marked **[OPEN]** need an answer before the matching plan task starts.

> **Update 2026-10-22:** the TUI design section below is replaced by `docs/superpowers/specs/2026-10-22-tui-v2-design.md`, which also amends D5 for API-key providers. Provider decisions D1 to D8 still apply.

## Goal

Manage accounts for GCP, AWS, Azure and Tencent Cloud from one tool and one TUI, with the same operations for each: create, login, switch, status (including expired credentials), run, shell, delete.

## Decisions

| # | Decision | Choice |
|---|---|---|
| D1 | Config layout | One `config.json`, each account gets a `provider` field. Missing `provider` means `gcp`, so existing files keep working with no rewrite. |
| D2 | Account names | Globally unique across providers. Simplest for `gctx switch <name>`; no `provider/name` syntax. |
| D3 | Provider abstraction | `pkg/provider.Provider` interface. GCP logic moves behind it unchanged; `manager.Manager` becomes a dispatcher. |
| D4 | Switching semantics | GCP changes global gcloud state (as today). AWS, Azure and Tencent are env-based: `switch` records the active account and prints how to apply it; `shell`, `run` and the new `env` command apply it. |
| D5 | Secrets | gctx never stores or prints secrets or tokens. Only names, profile names, ids and file paths go in `config.json`. |
| D6 | Expired-credential check | Each provider authenticates against its own CLI (`gcloud auth list`, `aws sts get-caller-identity`, `az account get-access-token`, Tencent TBD). Runs async with a per-provider timeout, never in `Overview`. |
| D7 | New name | **[OPEN]** `cctx` recommended. `gcc` is rejected: it is the GNU C compiler. `gctx` stays as an alias for at least one release. Done last; mechanical. |
| D8 | Tencent isolation | **[OPEN]** Depends on a spike (plan Task 8, step 1) against a real `tccli`. Design below assumes profile isolation exists. |

## Provider contract

```go
type Provider interface {
    ID() ID                       // "gcp" | "aws" | "azure" | "tencent"
    Label() string                // "GCP"
    TargetLabel() string          // "PROJECT" | "PROFILE" | "SUBSCRIPTION"
    CLI() CLIStatus               // {Bin string; Found bool}
    ValidateTarget(target string) error
    Create(ctx, acct config.Account) (CreateOutcome, error)
    Login(ctx, acct config.Account, io LoginIO) (LoginResult, error)
    Activate(ctx, acct config.Account) (ActivateResult, error)
    Local(acct config.Account) LocalStatus          // file reads only
    CheckAuth(ctx, accts []config.Account) AuthReport // may call the network
    Env(acct config.Account) ([]string, error)      // for run, shell, env
    Delete(ctx, acct config.Account, opt DeleteOptions) (DeleteResult, error)
    Details(acct config.Account) []Field            // provider-specific detail rows
}
```

Per-provider mapping:

| | GCP | AWS | Azure | Tencent |
|---|---|---|---|---|
| Target | project id | profile name | subscription id | profile name |
| Isolation (`Env`) | `CLOUDSDK_ACTIVE_CONFIG_NAME`, ADC path | `AWS_PROFILE` | `AZURE_CONFIG_DIR=<gctx>/azure/<name>` | profile env (D8) |
| Login | `gcloud auth login` + ADC login | `aws sso login --profile P` | `az login` in the isolated dir, then `az account set` | `tccli` login or configure (D8) |
| Auth check | `gcloud auth list` | `sts get-caller-identity` | `az account get-access-token` | D8 |
| Global switch | yes | no | no | no |
| Extra column | ADC | none | none | none |

Health values reuse the existing set and add `CLI MISSING` (provider CLI not on PATH). A missing CLI affects only that provider's rows.

## TUI design

### Main screen (≥120 columns)

```
cctx  v1.0   ● prod-aws (aws)   cfg: ok   env: AWS_PROFILE=prod
─────────────────────────────────────────────────────────────────────────────
 [All 12]  GCP 9   AWS 2   Azure 1   Tencent 0
Accounts (12)
  CLOUD  NAME                     TARGET                    EMAIL / IDENTITY            CRED  STATUS
  GCP    cloudsuite               sap-gap-cloud-asap-s      wahyu.anggana@emarsys.com    ✓    READY
  GCP    sbx-aliz                 sandbox-wahyu-anggana     wahyu.anggana@aliz.ai        ✓    AUTH REQUIRED
● AWS    prod-aws                 prod-sso                  arn:aws:sts::1234:role/dev   –    READY
  AWS    legacy-aws               legacy                    –                            –    AUTH REQUIRED
  AZR    corp-azure               8c1f…e2a9                 wahyu@corp.onmicrosoft.com   –    READY
─────────────────────────────────────────────────────────────────────────────
tab cloud  j/k move  ↵ switch  / filter  d details  l login  c create  e env  s shell  r run  ? help  q quit
```

- **CLOUD** is a colored 3-letter badge: GCP blue, AWS orange, AZR cyan, TCT green. Color is never the only signal: the letters carry it.
- **TARGET** replaces PROJECT; the header text per provider comes from `TargetLabel()`. When a cloud tab is selected the header shows that label (PROJECT, PROFILE, SUBSCRIPTION).
- **CRED** is the ADC column. It shows `✓`/`✗` for GCP and `–` for providers with no equivalent.
- The top bar shows the active account with its cloud. The `adc:` indicator appears only when the active account is GCP; otherwise it shows the env hint (`env: AWS_PROFILE=prod`).
- The cloud tab row is a filter, not a different screen. `tab` / `shift+tab` cycle All → GCP → AWS → Azure → Tencent. `1`–`5` jump directly. A tab with a missing CLI is dimmed and labeled `(not installed)`.

### Narrow (<90 columns)

The CLOUD column folds into the name as a prefix badge: `AWS prod-aws`. EMAIL and CRED are hidden, as today.

### Create form (`c`)

```
Create account

  Cloud     ‹ GCP │ AWS │ Azure │ Tencent ›     ← / → to change
  Name      ______________________
  Profile   ______________________     (label follows the cloud)

  [ Create ]   [ Cancel ]
```

The cloud picker is the first field and defaults to the active tab (GCP on the All tab). The target label and validation message change with it (`ValidateTarget`). If the provider's CLI is missing the form shows the install hint and disables `[ Create ]`.

### Details (`d`)

Common rows (name, cloud, target, status, created) plus the provider's `Details()` rows, e.g. AWS shows the SSO start URL and region; GCP keeps its current ADC rows.

### Switch feedback (env-based providers)

After `↵` on an AWS, Azure or Tencent account the status line reads:
`active: prod-aws · apply with: eval "$(cctx env prod-aws)"  (or press s for a shell)`.
`e` copies nothing; it runs `env` output into the status area for one line, or opens the details screen for longer output.

### Help screen

Adds `tab`/`shift+tab`, `1-5` (cloud filter) and `e` (env) under Navigation and Actions. Generated from `keyMap`, so it stays in sync with the hint line.

## Out of scope

Other clouds, secret storage or a keychain integration, cloud-side resource management, and a plugin mechanism for third-party providers.

## Risks

- Tencent CLI behavior is unverified (D8). The plan front-loads a spike.
- AWS and Azure auth checks need the network; offline users see `AUTH REQUIRED` only after the timeout, not before. The UI shows `checking…` meanwhile.
- Renaming the binary breaks existing shell aliases, completions and the Homebrew formula; handled in the final task with an alias.
