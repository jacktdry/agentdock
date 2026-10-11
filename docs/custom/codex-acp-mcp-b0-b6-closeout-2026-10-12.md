# AgentDock Next / Codex ACP MCP lifecycle — B0–B6 release-gate audit (2026-10-12)

## Decision / release boundary

B0–B6 is the fixed, non-expanding scope of this MCP cleanup workline. It concerns **Next-owned Codex ACP Adapter → dedicated native app-server → instrumented stdio MCP** only. Direct official Codex CLI, other apps, external MCP, HTTP MCP, CBM shared daemon and Chrome DevTools broker remain out of scope. Official Codex CLI Rust is not modified. Never infer that a native MCP belongs to one Codex Thread from a server name, PID, log or model reply. No blanket kill/pkill/sweep is acceptable.

**Final deployment status: NOT RELEASED.** The isolated engineering implementation and deterministic fixture UAT are validated; the authenticated end-to-end Next-only release gate is still blocked. This must not be portrayed as a completed production B6 or an auto-cleanup feature currently running. No currently installed Next Core, original AgentDock, global ~/.codex config or external MCP processes were changed. Commits are local only.

| Stage | Scope | Engineering validation | Release gate |
|---|---|---|---|
| B0 | Codex ACP task-end MCP cooperative reporting Prompt, explicit unknown | Source-only opt-in + private Adapter fixture, earlier commit `bdabe3e6` | Not installed |
| B1 | Pre-MCP launch intent + kernel PID/birth identity and transparent stdio | Source-only wrapper + nonce, replay/forgery rejection, `bfc109ac` | Explicit per-server config required |
| B2 | Private Core listener, strict parent chain, signed Adapter→native provenance | 5-process native OS fixture, `9d67aea9` | Not connected to production |
| B3 | Pre-native `CODEX_HOME` isolation and no-loaded-Session signed native diagnostic | Codex ACP fork `8487245`, Next `a45c73d1`; 12+ tests | Isolated authentication not provisioned |
| B4 | Start Adapter, verify signed native generation **before first Session**, pin Core listener snapshot and inject fingerprint-bound stdio wrapper into ACP mcpServers while preserving host HTTP MCP | Source-only Core bootstrap, GenerationPin with kernel resampling, no live Adapter round trip during native MCP startup; native Codex app-server with empty isolated CODEX_HOME initialized/config-read in private smoke; Next-only test provider | Requires actual P5/M8 allowlist admission + verified credential-bearing Next profile; no production startup or user authentication |
| B5 | Multi-Session logical leases, generation fence, read-only operator reconciliation, no native PID forced-close | 8 parallel logical ACP lease test, graceful ReleaseSession to existing host capability, same server collision fails, Core registry commit occurs **before** `recorded` reply; exited/reused native PID never permits cleanup | Native Thread/stdio transport owner remains opaque; return `unknown_or_deferred`; only existing ACP `session/close`/idle app-server recycle may gracefully shut down a verified idle dedicated app-server, not task-end auto-kill |
| B6 | Deterministic negative/stress and release-gate decision | 20 owned real stdio MCP fixture launches with parent/birth verification; 8 parallel real subprocess launches; cancellation, unsupported/forged config, nonce/replay/parent/UID, recycled generation, borrowed Sessions. Go full/race/vet/cross-build and TS typecheck/build/test checks recorded in the B6 execution log | **BLOCKED** until real Next-only authenticated session-to-MCP lifecycle, P5/M8 cross-Core policy/lease authority, live App UAT, and migration sign-off |

## B4 startup invariant

1. An explicitly selected, opt-in Next-only Codex ACP profile uses a dedicated **direct native Mach-O** CODEX_PATH, rather than npm JS launcher or symlink, so OS PPID lineage is meaningful. The binary is *never* modified. The isolated CODEX_HOME cannot contain a user/global MCP config.
2. Core explicitly calls `PrepareNextCodexMCPRegistry` **before** creating the first Session. `BootstrapNextCodexAppServerBirths` starts its owned Adapter, performs one authenticated nonce/MAC extension request and kernel birth observations of native app-server. No Session is fabricated.
3. Core pins that signed/native-scoped generation in `GenerationPin`, opens a private one-server socket, and holds it for the dedicated app-server generation. Wrapper handshake uses only this nonblocking snapshot and kernel rechecks. No adapter RPC from MCP init thread; no potential thread/start→MCP→ACP diagnostic cycle.
4. Trusted Core caller must first present a preapproved `ApprovedNextCodexMCPConfig`. `NextCodexMCPConfigDigest` binds server ID + backend absolute path + args; backend/wrapper executable paths must be absolute, regular and executable, not symlinks. `AttachNextCodexInstrumentedMCP` constructs the stdio MCP request with Core's signed birth and private registrar socket metadata; individual ACP Sessions retain other host-owned HTTP MCPs. A model prompt cannot modify the whitelist.
5. **Caveat**: the `ApprovedNextCodexMCPConfig` type is an internal caller contract, **not yet an integrated P5/M8 permission token**. Do not enable in production until a genuine cross-process admission/deny and immutable-generation policy store is wired. Current `Runtime` does not call these B4 methods automatically.

## B5 native cleanup decision

A verified `Core → Adapter → native app-server → wrapper → child` process chain is only *app-server-origin evidence*. Native app-server can host multiple threads that share MCP transport; host-managed HTTP MCP may have other clients. B5 therefore **never issues kill/pkill or a direct PID signal**. A `ReleaseSession` only decrements logical Core-owned references and releases its existing provider resource; if underlying release fails it retains the lease. Once all relevant Sessions have actually closed, the already implemented ACP Adapter `recycleAppServerIfIdle` may gracefully release its own dedicated app-server; we do not invent per-task native MCP ownership from this behavior.

Read-only `Reconcile` labels `still_in_use`, `observed_exited_or_replaced` or `unknown_or_deferred`, always sets `unknown_native_borrowers=true` / `cleanup_allowed=false`, and does not conflate process absence with successful intentional cleanup. The registry `AcceptWithCommit` records its final Core generation proof *before* responding `recorded`, preventing a false positive ACK after scope changes.

## B6 evidence and blockers

- Twenty deterministic, owned stdio child startups; each validates real kernel parent/wrapper/child birth and exact unchanged `mcp:hello` stdout. Separate eight-concurrent startup test; wrong config/birth/nonce/parent and canceled context tests. These are **engineering tests**, not invented production sampling.
- Real native Codex **app-server** smoke with a private temporary CODEX_HOME (without copying auth.json) confirms `initialize` and `config/read` with zero inherited MCP. This is a smoke test of the native ACP transport, not full authenticated `session/new`/`turn/start`/`session/close` with instrumented MCP.
- Go full regression, vet, new-path race, CGO-disabled Windows/Linux crossbuild and Codex ACP TS typecheck/build/related tests are mandatory before source-only merge. Windows/Linux cross-build is **not** native Windows/Linux UAT; feature code remains macOS only.
- Cannot claim full B6 release while authenticated Next-only profile sign-in/cookie/Keychain handling is unproven, there is no real immutable P5/M8 control admission token linked to wrapper config and cross-Core lease, and no authenticated MCP-through-Codex end-to-end UAT. No credential copy or login popup will be auto-triggered to force a green gate.

**Safe closeout statement**: B0–B6 source-only development + controlled fixture validation finished; **B6 production release gate remains blocked**. A future dedicated release integration pass must satisfy these concrete gates, but this file must not be rebranded as "B7" or used to justify extending the scope indefinitely.
