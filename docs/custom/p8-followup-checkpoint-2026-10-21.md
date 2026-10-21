# AgentDock NEXT P8 — 2026-10-21 verification and remaining product gates

> Source of truth for the *historical* three-actor acceptance: [P8 §52, 2026-10-14 installed NEXT three genuine actors](pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md#52-2026-10-14-p8-installed-next-three-real-actor-shared-chrome-overlap--owned-release-uat-pass). This checkpoint corrects a stale 2026-10-21 backlog summary; it does **not** claim a new three-real-actor live UAT.

## User workflow, current result and control path

The user wants a real ChatGPT + Codex ACP + Antigravity ACP workflow through **one NEXT Browser Broker**, reusing headless Chrome safely without interrupting the desktop. **All investigation, execution and file changes on this date went through Stable Mac-Dev only**; NEXT connector was not invoked as a mutation/control plane, Stable was not modified/restarted, and neither the NEXT Core nor NEXT GUI was restarted.

On 2026-10-21 NEXT source `custom/main` started at `af538822` (after the worktree closeout). Installed `~/Applications/AgentDock Next.app` helper reported **0.9.2-rc.4** / commit **`5860a2b46dee`**, arm64. Stable `127.0.0.1:8765/healthz` and NEXT `127.0.0.1:8767/healthz` both returned **HTTP 200**. NEXT Core's launchd description did not show `AGENTDOCK_NEXT_SHARED_BROWSER_WORKSPACE`; the restart marker was absent. Source confirms its **default is exclusive per lease**; shared mode remains an explicit **single-workspace, Next-only opt-in** requiring its exact canonical workspace and owner guard, not a general production default.

## Earlier live success that must not be regressed

On 2026-10-14, with reversible, NEXT-only temporary activation, **all three genuine actors** (ChatGPT first-party `browser_broker`, Codex ACP and AGY ACP) held simultaneous active leases using one real Chrome main process. Each evaluated and snapshotted its own marker, a cross-owner snapshot failed as `BROWSER_LEASE_OWNER_MISMATCH`, and AGY → Codex → ChatGPT released individually without breaking peers. Finally the shared slot recovered, owned worker stopped and NEXT private config was restored byte-for-byte to default exclusive. This gate is **PASS**, not pending. Distinguish it from older checkpoints when a third actor was blocked by quota or ChatGPT tool discovery.

## 2026-10-21 actual source-engine regression

Using **only Mac-Dev**, ran the repository's existing opt-in real test, without a stable/NEXT service restart or user browser/Edge access:

```sh
AGENTDOCK_TEST_REAL_SHARED_ACP_BRIDGE=1 NPM_CONFIG_OFFLINE=true \
  NPM_CONFIG_UPDATE_NOTIFIER=false GOPROXY=off GOSUMDB=off \
  GOMAXPROCS=2 GOFLAGS=-p=2 \
  go test ./internal/tool/browser -run '^TestACPBridgeSharedRealHeadless$' -count=1 -timeout=195s -v
```

- First run: **PASS**, actual `chrome-devtools-mcp@1.7.0`, 3 distinct *synthetic* ACPBridge owner identities and 2 SDK clients, **1 Chrome main per generation**, one occupied slot, isolated evaluate/snapshot/authorization, safe rotation and exact owned cleanup. Before/after pre-existing Chrome-related process count **8 → 8**; the test asserted its own MCP and Chrome process subtree was gone.
- Second run: **PASS** again with a separate read-only macOS `lsappinfo front` sampling observer (not GUI interactions, not screenshots). **96/96 foreground-app ASN samples matched the initial and final ASN, 0 observed switches**, while real Headless Chrome exercised the Broker. All test-owned subprocesses reported exited. This is evidence of *no observed foreground focus theft during this one bounded source test*, **not** proof of all native focus/Keychain/permission prompts in genuine installed ChatGPT/ACP or Edge workflows.
- The test names are *not* actual Codex/AGY/ChatGPT models; do not confuse this **source test** with the already accepted **2026-10-14 installed real three-agent UAT**.

Local evidence files: `/tmp/agentdock-next-p8-proof-20261021/real-shared-headless.log`, `native-foreground-real-shared.log`, `foreground-observation.txt`. These are local ephemeral logs, not public or durable release artifacts.

## What remains open

1. **P0 — User-visible route/operator configuration:** UI must explain truthful managed vs authenticated-Edge vs exact Chrome Profile routing, missing consent/credentials, stale provider health, safe recovery steps and strict no silent fallback. The existing Broker and conservative policy should be reused, not a second browser manager.
2. **P1 — consent-grounded company Edge / explicit Chrome CDP:** Production Edge still lacks fully qualified authenticated profile/consent/target routing live proof. Never start or modify the user's browser/profile/remote debugging without explicit valid authority, and do not equate endpoint/argv inspection with login proof.
3. **P1 — production native noninterruption:** Validate focus/Keychain/prompt behavior for actual installed NEXT *and* permitted Edge/Chrome browser actions, using non-destructive observation and controlled user consent; the source-engine ASN probe above covers only the safe headless subset. NEXT GUI currently had **no visible accessibility window**; the observer deliberately refused to restore/focus it.
4. **P2 / evidence-led — crash/kill/restart resource recovery:** Current restart guard fails closed with orphan evidence; SIGKILL/native crash reconciliation and reclaim remain unproven. Only modify shutdown/supervision architecture if a real user-flow blocker or S1/S2 ownership failure is demonstrated; preserve foreign processes and original AgentDock.
5. **P13/P14** remains separately unmerged, not part of P8 native acceptance. P10 official release still deferred.

No new source code was required for the already proven 3-actor Broker capability. The only changes in this checkpoint are correcting historical status and recording verified remaining gates. NEXT and Stable were not restarted or redeployed, no Tag/Release/Push.
