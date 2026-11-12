# ACP managed-session retention

Both AgentDock original and AgentDock NEXT use the same ACP manager code but keep
independent home directories and session stores. The retention policy is **seven
full days since `last_active_at`**, not seven days since session creation.

- Reuse an ACP session within a bounded task. Close it after verification and
  handoff when no further turn is planned. Closing preserves resume history.
- The manager's sweeper checks stale *AgentDock-owned mapping JSON* on startup
  and every 24 hours while running, using the existing manager and session store.
- Auto-pruning does **not** invoke adapter `session/delete`, and does **not** touch
  Codex/Antigravity native transcripts, standalone CLI history, or any foreign
  profile's storage. The adapter may continue to advertise native sessions.
- Auto-pruning skips a record marked `retention_protected`, records with running
  status, active prompt runs, pending permission requests, in-flight operations,
  active terminal transitions, and in-memory loaded sessions.
- Preview via `SweepStaleManagedSessions(now, true)`; apply with `false`.
  Both modes use the same eligibility checks; deleted mappings cannot be
  resumed using their old AgentDock `acps_*` identifier. A new mapping may be
  attached to the native conversation if the adapter still exposes it.
- No configuration UI, native transcript erasure, or cross-profile sharing is
  introduced. Do not directly edit active session JSON files.

The initial inventory on 2026-10-20 found 627 stale local mappings under
`~/.agentdock` and 150 under `~/.agentdock-next` (preview only). These counts
may change before the updated runtime is installed or restarted. Retention
is not active in already-running binaries until they are rebuilt and deployed.

## 2026-10-21 live rollout and handoff

The original and NEXT were deployed **sequentially**, with the opposite MCP
connection kept operational throughout each service restart. This is installed
runtime evidence, not a claim about future 24-hour sweeper executions.

| Runtime | Installed Core | Pruned on first start | Managed mappings left | Health |
| --- | --- | --- | --- | --- |
| Original / 8765 | `1.1.0` at `c0768a890d99` (source branch based on installed upstream `v1.1.0`) | Codex 539; Antigravity 102 = **641** | **37** | HTTP 200; original MCP reconnected |
| NEXT / 8767 | local-only `0.9.2-rc.4` at `5860a2b46dee` (includes live Orca PATH fix `35a69c74`) | Codex 98; Antigravity 59 = **157** | **8** | HTTP 200; NEXT MCP reconnected |

The original's first manual App hot-swap exposed a macOS `SMAppService`
`BundleProgram` registration failure (`EX_CONFIG 78`). Its old App was restored
and Core service recovered. Successful second rollout stopped the original Core
**through its GUI** (unregistering the old background job), quit only the
original GUI, installed the verified bundle, launched through LaunchServices,
then started Core through the new GUI. Avoid hot-swapping a still-registered
original App bundle. The NEXT rollout used its existing verified
`deploy-next-shared.py` transaction and GUI reopen; it reported
`NEXT_ONLY_DEPLOYED` with a retained rollback bundle.

Additional verification: original/Next native Codex transcript file counts
remained **59 / 113** before and after cleanup. NEXT observation-only ACP
status reported **0 loaded, 0 running, 0 auto-close failures** and no Broker
session leases. These are AgentDock mapping counts; native Codex and
Antigravity transcripts were **not** removed. Existing state backups are at
`~/Library/Application Support/AgentDock-Rollback/acp-retention-20261021/`
and `~/Library/Application Support/AgentDock Next-Rollback/acp-retention-20261021/`.
The verified NEXT transaction rollback Contents is under
`~/Applications/.agentdock-next-replacement-b9ciq9tg/AgentDock Next.app/`.

Tests: original full `go test ./internal/acp -count=1` passed on the installed
upstream `v1.1.0` source; NEXT `go test ./internal/acp ./internal/tool/computer
-count=1` passed, as did NEXT's 23 deployment-script tests and 9 bundle
validation tests. The NEXT candidate remains **unpublished**; no public tag,
release, enabled update feed, or formal production readiness is implied.

### Original Tunnel recovery verification (2026-10-21 12:57 +08:00)

The original macOS Tunnel LaunchAgent remained at `EX_CONFIG (78)` after
the Core was deployed. A public `GET https://mac-dev.dropabit.dev/healthz`
returned HTTP 200, but that alone did not establish ownership by AgentDock's
Tunnel: a separate system `com.cloudflare.cloudflared` service also runs.
Moreover, the current original CLI `tunnel status` implementation reports
`running` when the launchd job is *loaded*, even when its process failed to
spawn. Treat `launchctl list`, the owned process lineage and recent
`cloudflared` registration logs as the authoritative runtime evidence.

Recovery used the **original GUI Stop, then Start** (from the still-healthy
NEXT control channel). That unregistered/re-registered both original managed
services, without changing Tunnel mode/domain/token or the installed App.
Subsequent observation confirmed original Core PID `81757`, original Tunnel
PID `81765`, and its owned cloudflared child PID `81773`; Next Core and
Tunnel remained running. The original Cloudflare log recorded four fresh
registered connections around 12:57, and public `/healthz` remained HTTP 200.

**Follow-up, not part of this rollout:** fix the original macOS Tunnel status
readiness check so a loaded-but-spawn-failed launchd job is not reported as
`running/ready`. Do not alter this installed release just to change that
diagnostic; validate it separately. Keep rollback and recovery paths documented.
