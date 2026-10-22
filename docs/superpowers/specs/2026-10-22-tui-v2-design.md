# TUI v2: Clouds, AI Tools and AI Gateways: Design

Status: draft for review. Decisions marked **[OPEN]** or **[SPIKE]** need an answer before the matching plan task starts.

Builds on `docs/superpowers/specs/2026-10-21-multi-cloud-design.md` (D1 to D8). This spec:

- replaces that spec's "TUI design" section,
- amends D5 (secrets) for API-key providers,
- adds two provider categories: AI tools (Claude Code, Codex) and AI gateways (OpenRouter, SumoPod, any OpenAI-compatible endpoint).

Visual reference: `docs/superpowers/design/tui-v2/` (PNG screenshots and static HTML exported from the "cctx TUI: long-term design" canvas; see its README for known differences). Screen names below (`Main`, `GatewayTab`, ...) match the file names there. The canvas is a mockup: where it and this spec disagree, this spec wins. Examples use the name `cctx`; D7 is still open.

## Goal

One TUI to see, switch, log in to and run with every account a developer uses: cloud accounts, AI coding tools and AI API gateways. Same verbs for all of them, the same status vocabulary, and no secrets on disk.

## Decisions

| # | Decision | Choice |
|---|---|---|
| D9 | Active account | One active account per **slot**, not one global active account. A slot is the set of env vars or global state a provider owns: `gcp`, `aws`, `azure`, `tencent`, `claude`, `codex`, `openai-api`. All OpenAI-compatible gateways share the `openai-api` slot because they all set `OPENAI_BASE_URL` and `OPENAI_API_KEY`; activating one replaces the other. Config gains `active_by_slot` (map slot to name); the legacy `active_account` is still read and written as the `gcp` slot so an older binary keeps working. |
| D10 | Provider categories | `Category()` is `cloud`, `ai-tool` or `ai-gateway`. Categories drive grouping, badge style and which create-form fields appear. No other behavior branches on category: everything else goes through the provider contract. |
| D11 | AI tool isolation | Env-based, like Azure. Each AI-tool account owns `<config>/ai/<name>/` and the provider points the tool at it: `CLAUDE_CONFIG_DIR` for Claude Code, `CODEX_HOME` for Codex. **[SPIKE]** confirm both variables and a non-interactive login-status check per tool before its task starts. |
| D12 | Linked accounts | An AI-tool account may run on a cloud account instead of a subscription login: `settings.via = <cloud account name>` plus `settings.backend = vertex\|bedrock` and `settings.region`. Its status is inherited from the linked account (shown with `↳`), `l` logs in the linked account, and its env is the linked account's env plus the tool's (for Claude Code on Vertex: `CLAUDE_CODE_USE_VERTEX=1`, `ANTHROPIC_VERTEX_PROJECT_ID`, `CLOUD_ML_REGION`; on Bedrock: `CLAUDE_CODE_USE_BEDROCK=1`, `AWS_REGION`). Deleting a cloud account that something links to is refused and names the dependants. |
| D13 | Secrets (amends D5) | cctx still never stores, logs or prints a secret. API-key providers store a **key reference** instead: `op://vault/item/field` (1Password CLI), `gcpsm://project/secret[/version]` (GCP Secret Manager), `keychain:<service>` (macOS Keychain / Secret Service), `env:<VAR>` (a variable the user sets). The key is resolved only when starting a child process (`run`, `shell`, auth check) and lives only in that child's env. `cctx env` prints the lookup command (`export OPENAI_API_KEY="$(op read 'op://...')"`), never the value. Pasting a raw key is not supported. |
| D14 | Unverifiable status | A timeout or network error never becomes AUTH REQUIRED. It becomes UNVERIFIED with the last good result and its age. Last results are cached in `<cache>/status.json` (names, health, timestamps, identity, budget: no tokens or keys) so the first paint after launch is not blank. |
| D15 | Budget | Gateways report spend and limit when the endpoint exposes them. Below 10 % remaining the health is BUDGET LOW. Budget replaces EXPIRES in that column. |
| D16 | Command palette | `:` opens a fuzzy palette over every action and account. Each entry shows its CLI equivalent. New features add palette entries first and single-key bindings only when they earn one. |

## Provider contract additions

Added to the contract from the multi-cloud spec:

```go
type Category string // "cloud" | "ai-tool" | "ai-gateway"

type Provider interface {
    // ...existing methods (ID, Label, TargetLabel, CLI, ValidateTarget, Create,
    // Login, Activate, Local, CheckAuth, Env, Delete, Details)...
    Category() Category
    Badge() Badge                 // {Text: "AWS", Style: BadgeFilled|BadgeOutline|BadgeDashed, Color: ColorRef}
    Slot() string                 // D9; usually equals string(ID())
    Columns() []Column            // provider's own column for single-provider tabs, e.g. REGION, AUTH, BUDGET
    Form() []FormField            // create-form fields after Kind/Provider/Name (labels, validation, defaults)
}

type AuthResult struct {           // one per account inside AuthReport
    Credentialed bool
    Identity     string            // email, ARN, org; never a token
    ExpiresAt    time.Time         // zero: unknown
    Budget       *Budget           // gateways only
    Checked      time.Time
    Err          error             // timeout or network: becomes UNVERIFIED (D14)
}

type Budget struct{ Spent, Limit float64; Currency string } // Limit 0: unlimited
```

`Runner` gains `WithEnv(extra []string)` so a provider can inject a resolved key into one child process. Key resolution lives in `pkg/secretref` (`Resolve(ctx, ref string) (string, error)`, `ShellLookup(ref string) (string, error)`), used only by gateway providers and never logged.

## Health

Existing values stay; new ones are added. Every state has a glyph, a word and a colour, and the word alone is enough.

| Glyph (ASCII) | State | Source | Meaning |
|---|---|---|---|
| `●` (`*`) | READY | network | provider accepted the credentials |
| `⚠` (`!`) | AUTH REQUIRED | either | never logged in or logged out |
| `✗` (`x`) | EXPIRED | either | token or session past `ExpiresAt` (new) |
| `?` (`?`) | UNVERIFIED | network | check timed out; last good result and age (new, D14) |
| `○` (`-`) | CLI MISSING | local | provider binary not on PATH (from multi-cloud spec) |
| `⚠` (`!`) | BUDGET LOW | network | gateway below 10 % of its limit (new, D15) |
| `!` (`!`) | CONFIG MISSING, ADC MISSING, PROJECT MISMATCH, ERROR | local | unchanged |
| spinner | checking… | UI only | a check is in flight; not a `Health` value |

Linked AI-tool accounts show the linked account's health followed by `↳`.

## Layout

Regions, top to bottom: header, tab row, body (list, plus detail pane when wide), status line, hint line. Overlays (forms, login, palette, delete, help) draw over a dimmed body.

**Header.** App name and version, then one slot per D9 in category order: `GCP ● cloudsuite  AWS ● prod-aws  AZR ○ │ ai CLD ● claude-work  OAI ● codex-work  API ● sumopod-dev`, then config health and the age of the last auth check. A second, dim line shows the env the active env-based slots would export. `○` means the provider has accounts but none is active; providers with no accounts get no slot.

**Tab row.** `All N`, then one tab per provider with accounts, a `│` between categories. A tab whose CLI is missing is dim and labelled `n/a`. `tab`/`shift+tab` cycle, `1`-`9` jump. The tab is a filter combined with `/`, not a separate screen.

**List.** On `All`: grouped by category (`CLOUDS`, `AI TOOLS`, `AI GATEWAYS`), then by provider, each group heading showing the provider's target label and a count summary (`9 · 7 ready, 1 expired, 1 login`). `z` folds a group; a folded group shows only rows that need action plus `… N more ready`. On a single-provider tab the KIND column drops and the provider's `Columns()` appear.

Columns, in drop order as width shrinks (first dropped first): EXPIRES/BUDGET, IDENTITY, ADC, provider extra column, TARGET (middle-ellipsis). NAME and STATUS never drop. Marker column: cursor bar plus `●` in the slot colour for the active account of that slot.

**Detail pane** (≥150 cols, toggle `p`): badge, name, active state, `Details()` rows, linked account (D12), the env that activation applies (values for non-secrets, lookup commands for key refs), and the account's actions. Below 150 cols `d` opens the same content full screen (existing details screen).

**Status line.** Per-provider check progress (`✓ AWS 2/2  ⠼ Azure 0/1 (timeout 8s)`) and totals (`19 accounts · 1 expired · 3 need login · 1 low budget`). Transient messages reuse the existing `statusMsg` mechanism.

**Hint line.** Generated from `keyMap`, trimmed to what fits: `tab provider  j/k move  ↵ activate  / filter  l login  L login all stale  c create  s shell  : commands  ? help  q quit`.

**Breakpoints.**

| Width | Behavior |
|---|---|
| ≥ 150 | list + detail pane, full hint line |
| 100-149 | list only, `d` for details screen, KIND drops on single-provider tabs |
| 60-99 | badge folds into NAME (`AWS prod-aws`), tab row collapses to one cycling chip, header slots collapse to coloured dots, 5 hints |
| < 60×12 | "terminal too small (needs 60×12)" (existing `minWidth`, `minHeight`) |

## Screens

| Screen (canvas) | Behavior |
|---|---|
| Main, MainAI | as in Layout |
| Standard, StandardAI, GatewayTab | single-provider tab; after `↵` on an env-based account a feedback panel says the slot changed (and what it replaced), that the current shell is unchanged, and offers `y` copy eval line, `s` subshell, `r` run. For AI tools it warns how many processes of that tool are already running (they keep the old account). |
| Narrow | 60-99 cols, see breakpoints |
| Create, CreateAI, CreateGateway | `c`. Fields: Kind (Cloud, AI tool, AI gateway), Provider, Name (validated live, unique across all providers), then `Form()` fields. Clouds: target and auth fields (AWS: SSO or existing profile, start URL, regions). AI tools: auth = subscription or via cloud account (lists cloud accounts that support a backend, with their health). Gateways: base URL (preset per provider), key source (D13), exports (default `OPENAI_BASE_URL`, `OPENAI_API_KEY`, plus optional provider-specific names), default model (from `GET /v1/models`), and a Test step that resolves the key, checks auth and budget, and runs one tiny completion. Every form ends with "writes: ... / never: ..." lines. A missing CLI shows its install hint and disables Create. |
| Login | `l`. Full screen: breadcrumb, numbered steps with state, the provider's device code or URL large with `o` open and `y` copy, streamed CLI output on the right (`v` full log), `esc` cancels and kills the child (account unchanged). `L` queues every account needing login; `n` skips to the next. |
| Palette | `:`. Fuzzy over actions and accounts; right column is the CLI equivalent; `↵` run, `tab` complete. Replaces the reserved `Command` binding. |
| Delete | `ctrl+d` (existing binding). Lists what will be removed with checkboxes (config entry always; isolated dir; provider logout first), what is kept (cloud resources), warns when the account is active, and requires typing the name. Focus starts on Cancel. Refused when other accounts link to it (D12). |
| Help | `?`. Three columns from `keyMap.groups()` plus a providers block: CLI version or install hint per provider, config and log paths. |
| States | First run: detect gcloud configurations, `~/.aws/config` profiles and Azure subscriptions and offer to import by name. Tab with missing CLI: centred explanation and install command, `R` rescans PATH. Offline: banner, UNVERIFIED rows with age, local checks still run, switching still works. |
| Tokens | Visual language below |

## Keys

Existing bindings keep their keys. New ones:

| Key | Action | Note |
|---|---|---|
| `tab` / `shift+tab` | next / previous provider tab | new |
| `1`-`9` | jump to tab | new |
| `p` | toggle detail pane | new |
| `z` | fold / unfold group | new |
| `!` | only rows needing action | new |
| `o` | cycle sort: name, status, expiry | new |
| `L` | login every stale account (queue) | new |
| `e` | edit account (name, region, model) | new; multi-cloud plan used `e` for env, which moves to `y` |
| `y` | copy the env eval line | new |
| `S` | subshell with only this account (`s` gives every active slot) | new |
| `t` | test (gateways: one completion) | new |
| `>` | jump to linked account (D12) | new; `g` stays "first row" |
| `b` | open provider usage page in browser (gateways) | new |
| `:` | command palette | was reserved |
| `enter`, `/`, `esc`, `d`/`i`, `l`, `s`, `r`, `c`, `ctrl+d`, `a`, `R`, `?`, `q` | unchanged | `a` (save ADC) stays GCP-only and is hidden on other rows |

## Visual language

- Badges are three letters and carry the meaning; colour only reinforces. Clouds are filled (GCP `#6ea8fe`, AWS `#ffa657`, AZR `#56d4dd`, TCT `#c39bff`; Tencent is violet so it never reads as READY green). AI tools are outlined (CLD `#e3b38c`, OAI `#b9c7d6`). Gateways are outlined with a dashed look; in a terminal use `[ORT]`-style brackets in the badge colour when an outline cannot be drawn (ORT `#a3b1ff`, SMP `#e8a6c8`).
- Status colours: READY `#3fb950`, warnings `#d29922`, errors `#f85149`, dim `#8b949e`.
- Colour profile from lipgloss/colorprofile: truecolor uses the hexes above, 256-colour uses the nearest index, 16-colour maps clouds to bright blue, bright yellow, bright cyan, bright magenta, and the cursor becomes reverse video. `NO_COLOR` keeps the letters and adds brackets. `GCTX_ASCII=1` (and later `CCTX_ASCII`) keeps the ASCII glyph set from `styles.go`.
- Light terminals: the same roles with an inverted ground, detected with the profile's background query; `CCTX_THEME=light|dark` overrides.

## Spikes before coding

1. **Claude Code** (D11, D12): confirm `CLAUDE_CONFIG_DIR` isolates login state, find a non-interactive "logged in?" check, confirm the Vertex and Bedrock env vars, and whether a running session re-reads its config dir.
2. **Codex** (D11): confirm `CODEX_HOME` and a login-status check.
3. **SumoPod** (D15): `https://ai.sumopod.com` is OpenAI-compatible. Check whether `GET /key/info` (LiteLLM proxy) works with a user key and returns spend and max budget. If not, auth check falls back to `GET /v1/models` and BUDGET shows `–`.
4. **OpenRouter** (D15): confirm the key-info endpoint path and its usage and limit fields.
5. **Tencent**: unchanged, multi-cloud spec D8.

Each spike writes its findings into this section and closes the matching **[SPIKE]** marker. If a spike shows the approach does not work, stop and ask; do not guess.

## Out of scope

Raw API key storage, a keychain written by cctx, proxying or metering API traffic, cloud-side resource management, AI tools without an isolatable config dir, plugins for third-party providers.

## Risks

- AI CLIs change fast; their env vars and status commands can break between releases. Each AI provider pins the versions it was tested with in `Details()` and reports `UNVERIFIED` rather than failing when its status command changes.
- Key references depend on helper CLIs (`op`, `gcloud`, `security`). A missing helper shows CONFIG MISSING with the helper name, not a generic error.
- More slots means a longer header; the narrow layout collapses them to dots.
