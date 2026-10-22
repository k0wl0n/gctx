# TUI v2 mockups

Exported from the "cctx TUI: long-term design" canvas on 2026-10-22. `png/` holds 2x screenshots, `html/` the static mockup source (open in a browser).

These are visual references for `docs/superpowers/specs/2026-10-22-tui-v2-design.md`. Where a mockup and the spec disagree, the spec wins. Known differences:

- Mockups show `x` for delete; the spec keeps the existing `ctrl+d`.
- Mockups show `g` for "jump to linked cloud"; the spec uses `>` (`g` stays "first row").
- Mockups use `▲` for warnings; the code keeps the existing `⚠` glyph from `pkg/tui/styles.go`.
- Account names, emails and numbers are sample data.

| File | Screen | Width |
|---|---|---|
| 01-Main | main screen, list + detail pane | ≥150 cols |
| 02-Standard | AWS tab, env switch feedback | 100-149 |
| 03-Narrow | narrow layout | 60-99 |
| 04-Create | create form (cloud) | |
| 05-Login | streamed login, queue | |
| 06-Palette | command palette | |
| 07-Delete | scoped delete | |
| 08-Help | help screen | |
| 09-States | first run, CLI missing, offline | |
| 10-Tokens | badges, health, colours, breakpoints | |
| 11-MainAI | main screen with AI tools and gateways | ≥150 |
| 12-StandardAI | Claude tab, runs-on column | 100-149 |
| 13-CreateAI | create Claude Code account linked to a cloud account | |
| 14-GatewayTab | SumoPod tab, budget, key ref | 100-149 |
| 15-CreateGateway | create gateway with key reference and test | |
