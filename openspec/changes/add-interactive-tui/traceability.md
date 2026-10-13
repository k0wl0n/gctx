# Traceability Matrix — add-interactive-tui

Maps every requirement in `specs/*.md` to an automated test or manual-matrix row.

Legend:
- **Auto** — covered by a Go test (package::TestName)
- **Manual** — covered by a row in the manual verification matrix in CONTRIBUTING.md
- **E2E** — covered by the test-plan.md end-to-end scenario

---

## specs/interactive-tui/spec.md

### Requirement: Screen layout

| Scenario | Coverage |
|---|---|
| Header reflects drift | Auto: `pkg/tui::TestHeaderShowsDriftWarning` (view_layout_test.go) |
| Inside an isolated shell | Manual: matrix row M-02 (shell env var) |
| ADC indicator variants | Auto: `pkg/tui::TestHeaderADCIndicator*` (view_layout_test.go) |
| Alternate screen, no mouse | Auto: `pkg/tui::TestAltScreenEnabled` (view_layout_test.go) |

### Requirement: Account table

| Scenario | Coverage |
|---|---|
| ASCII mode (`GCTX_ASCII=1`) | Auto: `pkg/tui::TestASCIIGlyphs` (view_layout_test.go) |
| Active account highlighted and sorted | Auto: `pkg/tui::TestActiveHighlightAndOrder` (view_layout_test.go) |
| No active account | Auto: `pkg/tui::TestNoActiveAccountState` (view_layout_test.go) |
| Color suppressed with NO_COLOR | Manual: matrix row M-03 |

### Requirement: Responsive layout

| Scenario | Coverage |
|---|---|
| Narrow terminal hides optional columns | Auto: `pkg/tui::TestNarrowLayout` (view_layout_test.go) |
| Shrink below minimum and back | Auto: `pkg/tui::TestResizeNoSelectionLoss` (view_layout_test.go) |
| Selection kept visible (G to last) | Auto: `pkg/tui::TestScrollKeepsSelectionVisible` (view_layout_test.go) |
| Too-small message shown | Auto: `pkg/tui::TestTooSmallMessage` (view_layout_test.go) |

### Requirement: Keybindings

| Scenario | Coverage |
|---|---|
| Navigation bounds (no wrap at top/bottom) | Auto: `pkg/tui::TestNavigationBounds` (view_layout_test.go) |
| Delete is on ctrl+d, not plain d | Auto: `pkg/tui::TestDetailsOpenOnD` (keys_bindings_test.go) |
| Reserved colon | Auto: `pkg/tui::TestReservedColon` (keys_bindings_test.go) |
| All other keybindings dispatch | Auto: `pkg/tui::TestKeyDispatch*` (keys_bindings_test.go) |

### Requirement: Filtering

| Scenario | Coverage |
|---|---|
| Filter by project | Auto: `pkg/tui::TestFilterByProject` (view_layout_test.go) |
| Typing j in filter appends character | Auto: `pkg/tui::TestFilterTypingJAppends` (view_layout_test.go) |
| Clear filter with esc | Auto: `pkg/tui::TestClearFilter` (view_layout_test.go) |
| Zero matches shows message | Auto: `pkg/tui::TestFilterNoMatchMessage` (view_layout_test.go) |

### Requirement: Details view

| Scenario | Coverage |
|---|---|
| Details for account without ADC | Auto: `pkg/tui::TestDetailsADCMissing` (view_layout_test.go) |
| Details overlays — open and close | Auto: `pkg/tui::TestDetailsOverlayOpenClose` (view_layout_test.go) |

### Requirement: Switch action

| Scenario | Coverage |
|---|---|
| Switch to sandbox | Auto: `pkg/tui::TestStartSwitch` (modals_switch_test.go) |
| Switch with missing ADC warning | Auto: `pkg/tui::TestSwitchWarningADCMissing` (modals_switch_test.go) |
| Switch error leaves previous active | Auto: `pkg/tui::TestSwitchError` (modals_switch_test.go) |

### Requirement: Operation concurrency

| Scenario | Coverage |
|---|---|
| Double action while busy | Auto: `pkg/tui::TestBusyGuard` (modals_switch_test.go) |
| Stale reload result dropped | Auto: `pkg/tui::TestStaleSeqDropped` (modals_switch_test.go) |
| Double ctrl+c force-quit | Auto: `pkg/tui::TestDoubleCtrlCQuit` (keys_bindings_test.go) |

### Requirement: Create flow

| Scenario | Coverage |
|---|---|
| Valid create | Auto: `pkg/tui::TestCreateFormSubmit` (modals_switch_test.go) |
| Validation errors | Auto: `pkg/tui::TestCreateFormValidation` (modals_switch_test.go) |
| Cancel create | Auto: `pkg/tui::TestCreateFormCancel` (modals_switch_test.go) |
| Authentication not completed | Auto: `pkg/tui::TestCreateChainToLogin` (modals_switch_test.go) |
| E2E create to login | E2E-07 (manual/test-plan.md) |

### Requirement: Delete flow

| Scenario | Coverage |
|---|---|
| Default delete keeps gcloud config | Auto: `pkg/tui::TestDeleteDialogKeepsConfig` (modals_switch_test.go) |
| Delete including configuration | Auto: `pkg/tui::TestDeleteDialogIncludesConfig` (modals_switch_test.go) |
| Enter right after opening does not delete | Auto: `pkg/tui::TestDeleteDialogDefaultCancelFocus` (modals_switch_test.go) |

### Requirement: Save ADC flow

| Scenario | Coverage |
|---|---|
| Save confirmed | Auto: `pkg/tui::TestSaveDialogConfirm` (modals_switch_test.go) |
| Cancel save | Auto: `pkg/tui::TestSaveDialogCancel` (modals_switch_test.go) |

### Requirement: Suspend and resume for interactive subprocesses

| Scenario | Coverage |
|---|---|
| Terminal restored after subprocess | Manual: matrix row M-04 |
| PTY smoke — process launches and exits | Auto: `cmd::TestPTY_LaunchesWithoutCrash` (pty_unix_test.go) |

### Requirement: Login flow

| Scenario | Coverage |
|---|---|
| Successful login | Auto: `pkg/tui::TestBeginLoginSuccess` (exec_test.go) |
| Ctrl+C during browser login | E2E-11 (manual/test-plan.md) |
| Status messages for login outcomes | Auto: `pkg/tui::TestHandleLoginDone*` (exec_test.go) |

### Requirement: Run flow

| Scenario | Coverage |
|---|---|
| Isolated run (gcloud output) | Manual: matrix row M-05 |
| Ctrl+C during run | E2E-11e (manual/test-plan.md); Auto: `pkg/tui::TestTokenize*` (tokenize_test.go) |
| Tokenizer rejects dangerous commands | Auto: `pkg/tui::TestTokenize*` (tokenize_test.go) |
| ADC missing hint shown | Auto: `pkg/tui::TestRunPromptADCHint` (exec_test.go) |
| History navigation | Auto: `pkg/tui::TestRunPromptHistory` (exec_test.go) |

### Requirement: Shell flow

| Scenario | Coverage |
|---|---|
| Shell round trip | Auto: `pkg/tui::TestBeginShell` (exec_test.go) |
| Non-zero exit appended to message | Auto: `pkg/tui::TestHandleShellDoneNonZero` (exec_test.go) |

### Requirement: Credential check and refresh

| Scenario | Coverage |
|---|---|
| External change picked up (R) | Auto: `pkg/tui::TestReloadOnR` (keys_bindings_test.go) |
| Input stays responsive during creds check | Auto: `pkg/tui::TestNavDuringCredsCheck` (keys_bindings_test.go) |
| Creds handling updates visible rows | Auto: `pkg/tui::TestHandleCredsUpdatesVisible` (keys_bindings_test.go) |

### Requirement: gcloud unavailable

| Scenario | Coverage |
|---|---|
| Switch without gcloud | Auto: `pkg/tui::TestGcloudGating` (keys_bindings_test.go) |
| s and a still work without gcloud | Auto: `pkg/tui::TestShellAndSaveNotGated` (keys_bindings_test.go) |

### Requirement: Empty and error states

| Scenario | Coverage |
|---|---|
| First run (no config.json) | Auto: `pkg/tui::TestEmptyState` (view_layout_test.go) |
| Corrupt config | Auto: `pkg/tui::TestErrorState` (view_layout_test.go) |

### Requirement: Status messages

| Scenario | Coverage |
|---|---|
| Error persists after 10 seconds | Auto: `pkg/tui::TestStatusExpiry` (keys_bindings_test.go) |
| Info messages expire after 5 s | Auto: `pkg/tui::TestStatusExpiryInfo` (keys_bindings_test.go) |

### Requirement: Help screen

| Scenario | Coverage |
|---|---|
| Help closes with q | Auto: `pkg/tui::TestHelpClosesWithQ` (view_layout_test.go) |
| Help shows all bindings | Auto: `pkg/tui::TestHelpScreenContent` (view_layout_test.go) |

### Requirement: Quit and terminal restoration

| Scenario | Coverage |
|---|---|
| Clean quit | Auto: `pkg/tui::TestQuitKey` (keys_bindings_test.go) |
| Terminal restored on panic | Auto: `pkg/tui::TestPanicRestoreDoesNotKillProcess` (security_test.go) |

### Requirement: Startup performance

| Scenario | Coverage |
|---|---|
| First frame under 100 ms with 50 accounts | Auto: `pkg/manager::TestOverview50Under100ms` (status_bench_test.go) |
| No subprocess before first frame | Auto: `pkg/tui::TestNoSubprocessBeforeFirstFrame` (view_layout_test.go) |

---

## specs/credential-safety/spec.md

### Requirement: Credential contents are never exposed

| Scenario | Coverage |
|---|---|
| Invalid ADC error text — no token echoed | Auto: `pkg/tui::TestSecretNeverRendered` (security_test.go) |
| No token-printing commands | Auto: `pkg/tui::TestNoClipboardPackageInDeps` (security_test.go) |

### Requirement: No clipboard access

| Scenario | Coverage |
|---|---|
| Copy keys unbound; no clipboard package | Auto: `pkg/tui::TestNoClipboardPackageInDeps` (security_test.go) |

### Requirement: Credential file permissions

| Scenario | Coverage |
|---|---|
| Saved ADC permissions 0600 | Manual: matrix row M-06 (POSIX only) |
| Debug log mode 0600 | Auto: `pkg/tui::TestDebugLogFileMode` (debuglog_test.go) |

### Requirement: Bounded credential deletion

| Scenario | Coverage |
|---|---|
| Tampered adc_path not deleted | Auto: `pkg/manager::TestDeleteADCPathOutsideStore` (manager tests) |
| Stale credentials not resurrected | Auto: `pkg/manager::TestCreateNoStaleADC` (manager tests) |

### Requirement: Save ADC requires explicit confirmation

| Scenario | Coverage |
|---|---|
| Cancel save leaves file unchanged | Auto: `pkg/tui::TestSaveDialogCancel` (modals_switch_test.go) |

### Requirement: User commands are executed without a shell

| Scenario | Coverage |
|---|---|
| Tokenizer prevents shell injection | Auto: `pkg/tui::TestTokenizeRejectsMetachars` (tokenize_test.go) |
| Tokenizer rejects Windows danger chars | Auto: `pkg/tui::TestTokenizeWindowsDangerChars` (tokenize_test.go) |
| Leading `gcloud` is stripped | Auto: `pkg/tui::TestTokenizeStripsLeadingGcloud` (tokenize_test.go) |

---

## specs/cli-commands/spec.md

### Requirement: gctx tui subcommand

| Scenario | Coverage |
|---|---|
| `gctx tui` launches TUI | Auto: `cmd::TestPTY_LaunchesWithoutCrash` (pty_unix_test.go) |
| `gctx tui --debug` accepted | Auto: `cmd::TestPTY_GCTX_DEBUG_Accepted` (pty_unix_test.go) |
| Non-TTY prints error message | Auto: `cmd::TestPTY_NonTTY_PrintsNonTTYMessage` (pty_unix_test.go) |

### Requirement: GCTX_NO_TUI suppresses bare-gctx TUI launch

| Scenario | Coverage |
|---|---|
| GCTX_NO_TUI=1 suppresses launch | Auto: `cmd::TestShouldLaunchTUI` (tty_test.go); `cmd::TestPTY_GCTX_NO_TUI_SuppressesTUI` (pty_unix_test.go) |
| CI env suppresses launch | Auto: `cmd::TestShouldLaunchTUI` (tty_test.go) |
| Dumb terminal suppresses launch | Auto: `cmd::TestShouldLaunchTUI` (tty_test.go) |

---

## specs/account-management/spec.md

All account-management requirements are covered by existing manager-level tests unchanged by this change. No new traceability entries are required.

---

## Uncovered requirements (deferred)

| Requirement / Scenario | Reason |
|---|---|
| E2E-12: Windows PTY smoke | Needs a Windows machine; manual-matrix row M-07 |
| M-04 terminal restore end-to-end | Needs a human to verify cursor and echo state |
| M-05 isolated run (gcloud output) | Needs real gcloud installed |
| M-06 ADC file permissions on POSIX | Needs a login that creates a real ADC file |
| Color column with NO_COLOR | Manual only; terminal color state is not inspectable in unit tests |
