# P13 / P14 UX scope decision — 2026-10-22

**Authority:** User's explicit preference after actual ChatGPT Host R1 and original AgentDock visual/source comparison. **Current product decision supersedes earlier proposals to build a pinned/persistent Task Monitor or default live large card.** This is a NEXT product-scope decision, not a retroactive claim about experiment test coverage.

## Adopted user workflow

- **Prefer the original AgentDock compact Task card style and actual use timing.** The user is satisfied with its concise, collapsible Task output, and reports that the earlier same-Task duplicate-card issue is already satisfactorily addressed in their current original workflow. Preserve that UX in NEXT instead of rebuilding it unnecessarily. Do not assert that an isolated NEXT or another Host configuration is already verified to have identical no-duplicate behavior; investigate only a real regression.
- Keep the existing persisted `task_manage` lifecycle (`create`, checkpoint, get, review, complete), one logical Task ID, and concise tool-result snapshots. The cards serve as **on-demand results**, not a promise that an older card stays visible at the bottom of ChatGPT.
- **No fixed-bottom/pinned card requirement.** If ChatGPT Host offers no safe, supported single-card sticky UI, accept the Host boundary. Do **not** add a fullscreen/thread entrypoint, PiP, split-pane dashboard, refresh daemon, extra MCP tool, new overlay or repetitive cards merely to simulate pinning.
- Progress of a ChatGPT task can be narrated at meaningful conversation milestones by the orchestrator independently of MCP cards. Do not assume MCP metadata can directly control ChatGPT's progress/timeline UI. Independent workers keep persisted Task state; a new monitoring surface is not an accepted requirement.
- **P14 MCP card noise reduction: monitor only / no implementation at present.** The user finds current noise tolerable. Preserve legitimate approval, failure and progress information. Do not suppress or change UI metadata without a freshly observed, material issue.

## Status of previous experiments

- P13 candidate implemented optional `AGENTDOCK_NEXT_CARD_UX_EXPERIMENT=p13`, `task_create` with a live, polling iframe. Separate private real ChatGPT Host R1 confirmed one card at 0 → 14 → 57%, correct step/condition expansion and cross-client Task state. However, a card bound to an old conversation message scrolls away as the dialog grows; **user chose the original compact UX over adding new persistent monitoring infrastructure**. The experiment is **validated as research but NOT SELECTED for default or formal NEXT integration**.
- P13 candidate branch and historical evidence may be retained for reference; **do not cherry-pick/merge the live Task card experiment, deploy it, enable the opt-in, create a new permanent tunnel, or build a new Task Monitor** based on prior P13 proposals. The installed NEXT at 8767 and stable original at 8765 remain unchanged.
- P13 private R1 synthetic Task was left incomplete at 5/7 when its time-bounded preview ended. A terminal/polling-stop check was never performed. **Do not fabricate its completion or recreate the expired preview merely to reach 100%.** This is a deliberately abandoned experiment, not a product-release blocker.
- P14's fail-closed experimental commits remain unmerged; no rollout or automatic card suppression.

## Reopen only on evidence

Reopen P13/P14 only if there is an observed NEXT regression against original compact cards, repeated same-Task cards, loss of critical status/approval/errors, objectively excessive card noise, or a newly explicit user need for an autonomous-monitoring view. The acceptance should then be a small, task-specific real ChatGPT Host fix, preserving original Stable/Next isolation, not a broad new platform milestone.

## Updated prioritization and handoff

Mark the former P0 P13/P14 UX work as **deferred / no active implementation**. The immediate NEXT priorities remain independently grounded user-visible blockers (e.g. Browser route/consent and remaining Skills/Codebase Memory integration). Leave old P13/P14 artifacts as historical records with clear pointers to this decision. No production code, stable AgentDock services, GUI, mainline, release or GitHub deployment was changed by the scope decision.
