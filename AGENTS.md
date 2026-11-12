# AgentDock Next — product-first repository working agreement

Applies to all repository changes, ChatGPT tasks and ACP agents. Read alongside `~/.agentdock/AGENTS.md` and the accepted contracts in `docs/custom/`; never weaken platform security, M8 authority, or stable/Next separation.

## Product job (why Next exists)

Make **ChatGPT + delegated Codex / Antigravity** reliably coordinate local MCP/ACP tools, browser and computer operations while the user continues normal work. Shared Core-owned permissions and resource lifecycle, simple cross-platform Vue UI, truthful execution status, bounded cleanup and low-maintenance features are the product; an elaborate test sandbox is not. **Next exists alongside stable AgentDock only so development cannot break the stable Mac-Dev control channel**, not to create a separate testing product.

## Mandatory before starting each task

Write down, in the task/checkpoint, **(1) user workflow/pain, (2) direct user-visible or runtime improvement, (3) simplest feasible implementation, (4) observable acceptance and non-interruption check, (5) what will NOT be done**. If no specific workflow is improved, stop, classify as optional research and choose the next actual product task. Identify which previously accepted milestone is advanced and what remains to close it. Read closest implementation and reuse it, including upstream when practical.

Priority order: **real useful workflow → focused fix for an observed blocker → required permissions/ownership safety → essential regression → documentation/diagnostics**. A passing fake-client/isolated simulation alone is never the end-user acceptance of ChatGPT, Codex and Antigravity using a shared browser. Do not repeatedly replace a real integration test with stricter synthetic harnesses.

## Constraints worth protecting

- **Non-interruption** means no focus stealing, unexpected dialogs, repeated permission/Keychain prompts, browser profile replacement, unbounded MCP/Chrome processes, or killing unrelated user apps. Respect OS consent; do not click through or suppress prompts. It does **not** mean prohibiting ordinary macOS services needed for a real app to function.
- **Stable/Next**: stable Mac-Dev is the sole mutation control plane; never edit/restart/replace the stable app, Core, port 8765, service, `~/.agentdock`, existing Memory or browser profile as a Next test. Next is isolated at port 8767, independent state/services/connector. Confirm identity before Next-owned mutations.
- **Authentication/routing**: company work must use the user's explicitly designated authenticated Edge profile when valid; otherwise fail with a useful explanation. General work defaults to AgentDock-managed headless Chrome. Explicit requested Chrome profile must not be silently substituted. Zero prompt/focus is an acceptance criterion, not permission to bypass access controls.
- **Resource sharing**: new ACP/ChatGPT tasks reuse the Broker and correctly scoped leases. Do not introduce a fresh Chrome/MCP launch for each agent if one owned instance suffices. Stop only independently verified owned resources and respect unfinished others.
- **Honest state**: GUI snapshot, token observation, MCP initialize, adapter session, browser launch and real agent workflow are separate evidence levels. A partial proof cannot be described as production-ready. Do not fake availability, health, consent or user-facing actions.

## Anti-drift stop rules

- When a test fixture or macOS containment workaround itself becomes the main task, ask: **does a real normal-use workflow require this?** If not, record the bounded blocker, keep existing user data/consent safeguards, and proceed to another independent product task. No default independent macOS account/VM, native-appserver protocol emulator, new privilege broker, or extra long-running daemon merely to satisfy an artificial fixture.
- Only increase security/isolation complexity against an evidenced S1/S2 risk (wrong page/profile, authorization leak, unsafe child cleanup) or explicit platform requirement; do not claim protection against a fully compromised local OS (out of v1 scope). Fail closed for genuine user-data/authority risk; do not impose unrelated synthetic constraints on routine operations.
- Before adding new components, tests, layers, workstreams or milestones, show the existing solution gap, expected product benefit and measurable exit condition. Prefer repairing an actual path over creating another abstract framework. A blocker in one optional UAT path must not stop all independent P6/P7/P8/M9 tasks.
- Every implementation checkpoint reports **what a real user can now do**, verified live vs offline evidence, remaining blocker, and the **next closest user workflow**. Do not loop endlessly through test-only commits.

## Near-term P8 and release path

1. Make ChatGPT + real ACP agents route background browser operations through the existing AgentDock Broker; verify one managed Chrome per engine/workspace, distinct leases/pages, no focus/prompts and only owned cleanup with a bounded real workflow.
2. Preserve truthful Browser operator UI, specific profile/authorization blockers and useful recovery steps. Existing Next and stable runtimes must remain operational during source work.
3. Treat restrictive native Codex Seatbelt `managed preferences` failure as a **test-environment-specific finding**, not a new platform/product milestone. Don't expand isolation just to make that test pass; instead use an appropriate genuine workflow acceptance path under existing legitimate credentials and controls.
4. After essential P8 acceptance, finish outstanding native GUI/parity evidence, ACP integration, updater/release tests and M9. Keep NexusDock deferred (`開發中`) per product choice.

Authoritative detail: `docs/custom/architecture.md`, `docs/custom/agentdock-next-isolation.md`, `docs/custom/pre-m9-browser-noninterrupting-routing-decision-2026-10-10.md`, and `docs/custom/next-product-first-guardrails.md`.
