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
