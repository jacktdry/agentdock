# M5 Activity / Execution Vertical Slice

> Status: Implementation complete — closeout verified
> Branch: `feature/activity-center`
> Last progress sync: 2026-10-03 19:50 Asia/Taipei

## Closeout summary

M5 now provides the product-facing Activity Center on the Shared Desktop shell. The synthetic Activity probe remains Developer-only diagnostic scaffolding and is no longer the product execution source.

Completed:

- Core owns the execution source of truth via a bounded in-process journal with stable root/child call IDs, explicit parent relationships, truthful `running` / `waiting_for_user` / terminal states, epoch/sequence/pruned-gap semantics and cancellation-aware waiting.
- Runtime loopback API exposes authoritative snapshot, bounded replay, direct-loopback/auth-protected SSE streaming, insertion control, expected-epoch validation and ahead-cursor reset.
- Shared Desktop resolves Core locally, consumes snapshot/replay/SSE through a Go-only client, and forwards bounded messages over Wails without exposing Core credentials to Vue.
- Product Activity Center uses a bounded Pinia/reducer path: 120 visible calls, 200 recent events and 128 insertion records, with active-call prioritization, parent/child presentation, explicit waiting-for-user attention, structural output/file facts and no fabricated reason text.
- User insertion has stable `insertion_id` and `accepted` / `delivered` / `rejected` / `expired` / `cancelled` states. ACK means Core acceptance only; delivery is emitted separately at a real successful root result boundary.
- Core explicitly exposes insertion eligibility. Observable child calls such as `command_session` and dynamic MCP children stay visible but cannot accept input unless they have a real delivery boundary.
- Dynamic MCP child lifecycle now distinguishes success, semantic `isError:true`, transport/tool failure and cancellation without changing the public MCP result/error contract.
- Accepted insertions expire while an otherwise idle healthy stream is waiting; expiry does not depend on a later snapshot or control action.
- Reconnect is fail-closed for controls. A snapshot updates authoritative state but does not re-enable insertion controls until the Core SSE connection is actually attached. Same-epoch replay may update last-known state while controls remain disabled.
- Core restart races are guarded by expected epoch plus cursor-ahead detection. Epoch mismatch or an old cursor beyond the new journal immediately emits reset rather than preserving stale running state.
- Insertion request envelopes are bounded for worst-case JSON escaping while preserving the advertised 8192-byte decoded UTF-8 limit.
- Execution history remains metadata-first: no raw stdout/stderr, unrestricted file contents, tool arguments/results, auth headers, environment maps, secrets or arbitrary remote MCP bodies enter the journal.
- Shared UI execution controls use native form controls/buttons, textual status and non-color-only waiting/failure semantics. Native VoiceOver/Narrator smoke remains release UAT; no code-level accessibility blocker was found in independent review.

Independent review initially reported six P2 findings and no P0/P1 findings. All six are fixed and covered by regression tests: insertion eligibility, MCP semantic/cancellation status, idle expiry, restart epoch/cursor race, reconnect synchronization state, and JSON envelope expansion.

## Verification

Closeout verification on 2026-10-03:

- `go test ./...` — pass.
- M5 packages `go vet` — pass.
- M5 packages `go test -race` — pass.
- Focused regression tests for MCP semantic failure/cancellation, epoch mismatch/ahead cursor reset, unsupported child insertion, idle expiry and escaped 8192-byte requests — pass.
- Shared POC Go tests — pass.
- Vue `vue-tsc --noEmit` — pass.
- Vitest — 30/30 pass across 7 files.
- Production Vite build — pass.
- i18n deterministic generation/check/coverage — 3 stable locales, 197/197 keys each.
- Wails bindings regenerated through the project task.
- macOS arm64 Shared build — pass.
- Windows arm64 Shared cross-build — pass.
- Windows amd64 Shared cross-build — pass.
- `git diff --check` — pass.
- Known non-blocking build note: macOS linker still reports the pre-existing deployment-target warning; no new native build failure was introduced.

## Goal

M5 replaces the synthetic Activity probe as the product-facing execution source with truthful AgentDock Core execution facts. The shared desktop process is a client; it must never invent execution state from recent UI interaction or from its own Wails process.

## Source of truth

The AgentDock Core process owns:

- stable call identity and parent/child relationships;
- call lifecycle state;
- bounded activity history and replay cursor;
- bounded, sanitized output facts;
- file-change facts;
- user insertion state and acknowledgement;
- explicit attention state.

The shared Wails/Vue shell consumes this through the existing loopback Runtime API. A stream disconnect is a transport failure, not an execution-state transition.

## Workbench donor decision

AgentDock-Workbench has a mature Execution Center and is used as a design donor, especially:

- append-only activity sequence semantics;
- stable CallID / ParentCallID;
- bounded output and redaction;
- insertion IDs and delivery state;
- snapshot + replay/SSE reconnect;
- explicit active/attention semantics.

M5 does **not** wholesale port its broader execution subsystem because that would prematurely import M6/M8 scope:

- automatic Conversation binding;
- Task thread lifecycle expansion;
- Workspace registry/routing;
- approval policy engine;
- archive/trash management surfaces.

Those can be adopted later behind the same M5 execution contract.

## Core model

### Execution status

The contract distinguishes:

- `running`
- `waiting_for_user`
- `completed`
- `failed`
- `cancelled`

No “recent interaction” timestamp may be used to infer `running`.

### Calls

Each real Runtime tool call receives a server-generated stable `call_id`.

Nested execution inherits the current call through `context.Context` and receives a new `call_id` plus explicit `parent_call_id`. Ordering is based on the execution journal sequence, not timestamps.

### Activity journal

The initial M5 journal is an in-process bounded ring with:

- process epoch;
- monotonically increasing sequence;
- bounded event capacity;
- explicit pruned-through cursor;
- replay by `after` cursor;
- authoritative snapshot of live/recent calls.

A Core restart creates a new epoch. Clients must discard the previous epoch and reload a snapshot. M5 does not claim durable cross-restart history; Workbench's segmented disk store remains a later optional adoption.

### Runtime API

M5 extends the existing loopback Runtime API instead of adding a second desktop-only backend.

Runtime paths:

- `GET /internal/runtime/execution` — authoritative snapshot.
- `GET /internal/runtime/activity?after=<seq>&limit=<n>` — bounded replay page.
- `GET /internal/runtime/activity/stream?after=<seq>&epoch=<epoch>` — loopback/auth-protected SSE stream with expected-epoch and ahead-cursor reset.
- `GET /internal/runtime/insertions?call_id=<id>` — bounded insertion state.
- `POST /internal/runtime/insertions` — enqueue/cancel control with strict request validation.

## Security boundary

Execution history is intentionally metadata-first.

Never persist or send to the shared UI by default:

- raw tool arguments;
- raw tool results;
- authorization headers or tokens;
- environment maps;
- unrestricted file contents;
- arbitrary remote MCP bodies.

File changes are represented as bounded structural facts. Output previews require explicit sanitization/truncation before entering the journal.

## Insertion

Insertion is an execution-domain object, not a Vue queue.

Each insertion has a stable ID and one observable state:

- `accepted`
- `delivered`
- `rejected`
- `expired`
- `cancelled`

ACK means the Core has atomically accepted the insertion into its bounded in-process execution state; it does not claim that an upstream model consumed it. Delivery is a separate transition and only insertion-capable calls may accept input.

## Reconnect rule

On connect/reconnect:

1. load authoritative execution snapshot;
2. compare epoch;
3. request replay after the last accepted sequence when the epoch matches;
4. if the cursor was pruned or epoch changed, reset local derived state from the snapshot;
5. then attach the live stream.

The UI must never preserve stale `running` state solely because the transport disconnected.

## Accessibility

Execution controls must be keyboard reachable and expose textual state. Waiting-for-user, failure and attention must not rely on color alone. VoiceOver/Narrator validation remains an M5 exit gate.

## Internal migration

The existing `ActivityProbeService` and Developer probe remain temporarily available while the real Core-backed execution path is built. They are diagnostic scaffolding and are not the M5 product source of truth.
