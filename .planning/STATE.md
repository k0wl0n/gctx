# Project State

## Current Position

**Milestone:** 1 — Stable Cross-Platform CLI
**Active Phase:** 1.1 — Fix gctx Login, Switch, and Shell Command Bugs
**Plan:** 01 of 01 in phase 1.1
**Status:** Phase 1.1 complete

Last activity: 2026-04-01 — Completed 01.1-01-PLAN.md

Progress: █░░ (Phase 1 done, Phase 1.1 done, Phase 1.2 pending)

## Accumulated Context

### Roadmap Evolution

- Project initialized with GSD planning structure on 2026-04-01
- Phase 1 retroactively marked complete (core implementation exists in codebase)
- Phase 1.1 inserted after Phase 1: Fix gctx Login, Switch, and Shell Command Bugs (URGENT) — COMPLETE
- Phase 1.2 inserted after Phase 1.1: Remove Auto-Save Flag - Always Auto-Authenticate on Create (URGENT)

### Decisions

| Decision | Rationale | Phase |
|----------|-----------|-------|
| Windows shell preference: pwsh.exe > powershell.exe > COMSPEC > cmd.exe | Prioritizes modern cross-platform PowerShell Core; gracefully degrades | 01.1-01 |
| ADC missing is a warning, not a fatal error in SwitchAccount() | Account config and gcloud config are still valid; only ADC consumers affected | 01.1-01 |

### Known Issues

- All 5 accounts have empty `adc_path` fields in config.json (users must run `gctx login <account>`)
- Binary needs rebuild to pick up fixes from 01.1-01

### Technical Notes

- Binary was last built 2026-01-05T06:34:30Z and is stale vs current source
- Login fix already exists in source (manager.go:129-133)
- StartShell() Windows fix: runtime.GOOS gate in pkg/manager/manager.go
- SwitchAccount() non-fatal ADC fix: pkg/manager/manager.go lines 221-225

## Session Continuity

Last session: 2026-04-01T08:23:13Z
Stopped at: Completed 01.1-01-PLAN.md
Resume file: None
