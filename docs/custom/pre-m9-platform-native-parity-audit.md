# Pre-M9 P7 Phase C — Native Shell / Platform Parity Audit

> 2026-10-08, read-only source and live-window evidence. **No Phase C implementation or native GUI acceptance is claimed.** This audit is deliberately independent of the concurrently active P7 Phase A2 Nexus backend work. Stable AgentDock, its Core, Tunnel, Memory and service registrations are out of scope and must not be changed.

## 1. Source-backed current state

| Surface | Verified implementation | Actual boundary / outstanding gap |
| --- | --- | --- |
| Shared Wails app menu + tray | `desktop/shared-poc/main.go` creates application menu with Show Window (`CmdOrCtrl+Shift+A`) and Quit, and native system tray with Show and Quit. Window Closing hook hides rather than destroys the window; `--background` starts hidden. | **Already exists.** Do not create a second tray/renderer menu. There is no Core health, Nexus state, OS permission or update action in this Shared tray. Adding these requires explicit native app/Next Core health ownership and truthful stale/unavailable status; never copy legacy privileged actions blindly. |
| Native window sizing | Same Wails window declares `MinWidth: 760`, `MinHeight: 560`; `normaliseWindowSize` also replaces persisted widths below 760. | **320 CSS px cannot be reached by manually resizing this native window.** Test CSS at 320px in isolated browser/devtools *and* decide whether to safely lower Wails minimum size for a separate native viewport acceptance. Do not report native 320px PASS on CSS-only evidence. |
| Actual Next GUI visibility | Live `orca computer list-windows --app dev.dropabit.agentdock.next --json`: process was running (PID 45275), windows `[]`; `get-app-state` returned `window_not_found`. | The cause (background flag, hidden window, other state) is **not established**. Do not claim an app crash or completed P6 picker/keyboard UAT. Restore via existing Show action only during a user-approved foreground GUI acceptance window. |
| Diagnostics Shared UI | `internal/desktopapi/diagnostics.go` snapshot: platform, architecture, runtime root, runtime directory and manifest availability; `desktop/shared-poc/frontend/src/components/system/DiagnosticsCard.vue` presents these plus raw absolute `runtimeRoot`. | No native file-open action. The **existing** absolute root is already renderer-visible, so proposed Phase C safe opener must not worsen disclosure (and product/security should decide whether existing root display should become an explicit masked/revealable diagnostic). Never return logs, environment or config file contents. |
| macOS legacy open folders | `desktop/macos/AgentDockApp/Sources/ServiceController.swift` `openLogs/openConfiguration` calls `createDirectory` and `NSWorkspace.shared.open` on app-identity-derived `paths.logs` / `paths.appSupport`. | **Legacy-only** action path. Next Shared Desktop does not expose it. A new Next opener needs revalidated Next identity, parent/directory ownership and symlink-race prevention; do not call legacy controller or reuse unverified directory paths. |
| Windows legacy open folders | `desktop/windows/control-panel/Services/RuntimeService.cs` `OpenLogsDirectory/OpenConfigDirectory` call `OpenDirectory`, which creates the directory then launches `explorer.exe`. | **Legacy-only** implementation, not Next Wails authority. New Windows adapter must deny reparse/junction escapes and check Next-owned runtime identity/ACL before create/open, then check native availability. Do not copy the existing create-and-open code as-is. |
| Native log path | `internal/desktopruntime/logging_darwin.go` derives `~/Library/Logs/<macOSRuntimeName>`; Next's distinct runtime name is already available through identity-aware backend. | Phase C logs destination should be **Next logs**, not stable's logs and not whatever path renderer supplies. Verify actual resolved identity at action time; no stable fallback. |
| Core startup | `internal/desktopapi/basic_settings.go` returns `CoreAutostartMutable` from `desktopruntime.BasicAutostartMutable()`; macOS Core autostart registration is native SMAppService. | The Shared Settings surface reports read-only/unavailable capability where the native adapter cannot mutate. Core autostart is **not** Wails app-login autostart, menu/tray login, or Tunnel autostart. Keep these separate. |
| Update | `internal/desktopapi/update.go` exposes **Check** only; `internal/desktopapi/contract.go` says native apply/recovery. | No Phase C override. Enabling Apply without M9 signed updater/release gates is forbidden. |
| Permissions | `internal/desktopapi/permission.go` and Shared `PermissionPanel` manage Core's confirmation/policy/approval state. | OS grants (Accessibility, Screen Recording, Automation, elevated service permissions) are different authority and must have platform-native capability / guidance; do not infer OS permissions from Core Allow/Ask/Deny. |
| Shared domain manifest | `internal/desktopapi/contract.go` enumerates Runtime, Connection, Settings, ACP, Browser, Activity, Permission, MCP, Plugin, Update and Diagnostics. No P7 Nexus domain in the committed manifest at audit time. | The concurrently active Phase A2 Nexus service is **not yet Wails-bound** at this audit checkpoint. Review negotiation/typing before Phase B; do not advertise readiness from unbound backend files. |

## 2. Recommended Phase C slice and ownership

### C1 — Native-only Next directory shortcut

Expose exactly `OpenNextDirectory(kind: 'logs' | 'configuration')` through a typed Desktop service; the renderer passes an enum, **never a path**, shell command or URL. Backend resolves Next-only runtime identity and its known directory destinations, verifies ownership, parent chain and non-symlink/reparse-point state *immediately before opening*, and fails closed on missing, foreign, ambiguous or changed identity. Return `completed`, `capability` and a fixed safe error code; return **no absolute path or file contents** from this new API. A user-triggered config shortcut warns that files may contain secrets.

The term **configuration** needs a frozen precise target: the Next App Support runtime root (`~/Library/Application Support/AgentDock Next`) differs from Next state home (`~/.agentdock-next`), and both differ from stable folders. Choose one explicit allowlisted location; avoid silently opening an unreviewed credential directory. Creating a missing directory is allowed only on explicit action after trustworthy parent/owner verification. A file-manager/open-system callback that resolves a pathname after validation must be assessed for TOCTOU symlink swaps; Windows reparse points and macOS parent swaps require native-path-specific tests, not only a string-prefix check.

### C2 — Platform capability and tray truthfulness

- Preserve Wails native menu/tray Show/Quit and native ownership of visibility; add useful Core health only if observable from verified **Next** Core, with unknown/stale states. Avoid duplicate Runtime lifecycle or shell-mediated Core restarts.
- Return explicit platform native capability/reason for opening logs/config and OS permission details. Unsupported Linux helper/WSL or native bridge must be **disabled with reason**, not silently converted into a guessed success.
- Keep Core autostart under Settings; Tunnel autostart under Connection; login item/tray startup separate. P7 is not an updater apply migration or privilege escalation project.
- Do not copy macOS AppKit or WPF business UI. No app menu action should use stable `agentdock` CLI, stable Core endpoint or global home defaults.

### C3 — Acceptance matrix

| Test | What proves success | Caveat |
| --- | --- | --- |
| Next tray/menu | Show from hidden/closed state; Quit exits; repeat with `--background`; App vs Next Core health accurately distinguished | No foreground activation during unattended testing without approval |
| Native windows + accessibility | Desktop at supported minimum; focus/keyboard/Escape; browser 320px CSS and optionally native 320px if supported after policy | Present `MinWidth:760` makes native 320px test inapplicable |
| Open logs/config | Only Next-owned fixed directory opens; safe warning for configuration; unknown kind denied | No arbitrary renderer path; no secret values in return/log |
| Directory race/ownership | Missing/foreign manifest, stable root selected, symlink parent/child swaps and Windows junctions all reject | Tests must use real OS-native adapters on supported platforms |
| Autostart/status | Core vs app login/tray vs Tunnel state and disabled reasons remain distinct | macOS SMAppService/native-only functionality is not automatically available to Wails |
| Permission/update | OS grants separately labeled from Core approval; Update remains check-only | Any release apply waits M9 gate |
| Isolation | Stable Core, Tunnel, Memory, app and service labels unchanged during test | Only test Next-scope state; no real pairing or updater mutation |

## 3. Gates / handoff

**Evidence established:** existing Wails tray is real; 760px native minimum blocks 320px resize; Diagnostics exposes runtime root but has no opener; native folder shortcuts shown in AppKit/WPF are legacy-only; security/per-platform semantics require new Next-native bindings. Committed Phase A1 `internal/nexusbridge` tests and vet were exercised again successfully in this session (`go test ./internal/nexusbridge -count=1`, `go vet ./internal/nexusbridge`).

**Still open:** P6 native GUI picker/focus/responsive acceptance; P7 Phase A2 authority/mutations (concurrent task), Phase B UI, Phase C opener + tray/platform UAT, Phase D independent review, and M9 release gate. Do not mark P6 or P7 complete based on this document. Read alongside `pre-m9-platform-essentials-roundtable.md`, which is the authority design source of truth; this audit records extra source evidence, not a competing contract.
