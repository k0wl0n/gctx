# gctx Roadmap

## Milestone 1: Stable Cross-Platform CLI

**Goal:** Deliver a working, cross-platform gctx binary that correctly handles login, account switching, and shell sessions on Windows and Unix.

---

### Phase 1: Initial Setup & Core Implementation

**Goal:** Establish the project foundation, implement core account management, and get the CLI functional.
**Status:** COMPLETE
**Plans:** 0 plans

Plans:
- [x] Core project structure established
- [x] Account management (create, list, delete)
- [x] gcloud config integration
- [x] ADC (Application Default Credentials) management
- [x] Shell session support (initial implementation)
- [x] Login flow implementation

---

### Phase 1.1: Fix gctx Login, Switch, and Shell Command Bugs (INSERTED)

**Goal:** Resolve critical Windows-blocking bugs in shell command, account switching, and login flow so gctx is fully functional cross-platform.
**Status:** COMPLETE
**Depends on:** Phase 1
**Plans:** 2 plans

Plans:
- [x] 01.1-01-PLAN.md — Apply cross-platform shell fix and non-fatal ADC switch fix to manager.go
- [x] 01.1-02-PLAN.md — Rebuild binary and verify all three fixes are active

**Details:**
Five bugs identified in `pkg/manager/manager.go`:

1. **Login fix already in source** — binary needs rebuild (built 2026-01-05, stale)
2. **Missing ADC directory** — all 5 accounts have empty `adc_path`; directory doesn't exist
3. **Switch requires ADC** — `SwitchAccount()` hard-fails if no saved ADC; should warn and proceed
4. **Shell fails on Windows** — `StartShell()` hardcodes `/bin/sh`; need `COMSPEC`/PowerShell fallback
5. **Active account shows "unknown"** — `SetActive()` never called because switch fails early (auto-fixed by #3)

**Priority:** Shell fix (critical) → Switch fix (high) → Rebuild binary (medium)

---

### Phase 1.2: Remove Auto-Save Flag - Always Auto-Authenticate on Create (INSERTED)

**Goal:** Simplify the `gctx create` UX by removing the `--auto-save` flag and always running authentication + ADC save automatically on account creation.
**Depends on:** Phase 1.1
**Plans:** 1 plans

Plans:
- [ ] 01.2-01-PLAN.md — Remove --auto-save flag and autoSave parameter, make auto-authenticate the default

**Details:**
Remove `--auto-save` flag from `cmd/create.go` and always pass `true` to `CreateAccount()`.

Key changes:
- `cmd/create.go`: Remove `autoSave` var, remove `init()` flag registration, hardcode `true` in call
- `pkg/manager/manager.go`: Optional — keep `autoSave bool` param (Option 2a) or remove it entirely (Option 2b)

After fix: `gctx create <account> <project>` always auto-authenticates, no flag needed.

---

