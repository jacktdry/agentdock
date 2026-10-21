# AgentDock NEXT — Worktree consolidation (2026-10-21)

> **Performed on Mac-Dev, read/write only in Git/source workspace; no Stable or NEXT service restart.** This is an operational checkpoint, not a release/tag or feature integration.

## Decision and safeguard

Original AgentDock is **Maintenance-only**. The NEXT developer branch is `custom/main`; stable `main` and the original-specific maintenance branch remain separate. User authorized safe clean-up of old `~/sideProject/agentdock-*` directories. We preserve all branch refs, two uncommitted worktrees, P10/P11 native release/UAT evidence, and unique unmerged feature branches. We do not reinterpret patch-equivalence as ancestry merge.

Preflight found **45** worktrees in the common AgentDock Git repository, a clean `custom/main` with **293** commits ahead of its locally recorded `origin/custom/main`, and two dirty worktrees. The original and NEXT returned HTTP 200 from their separate `/healthz` endpoints at 8765 and 8767. No process command line referenced the candidate source directories. Two independent repositories `AgentDock-Workbench` and `agentdock-docs` were not touched.

## Recovery before cleanup

`~/sideProject/.agentdock-archives/worktree-closeout-20261021/` contains:

- `agentdock-all-refs.bundle` (14 MB): Git **--all** ref bundle, checked with `git bundle verify`.
- `git-all-refs-before.txt`, `worktrees-before.txt`, `custom-head-before.txt`: original refs, worktree inventory and NEXT head.
- `dirty-worktrees/agentdock-m6-acp-tests/`: 17 changed/untracked source files plus binary-capable Git patch/status.
- `dirty-worktrees/agentdock-p5-mcp-oauth-fence/`: 5 changed tracked files plus patch/status.
- `evidence/agentdock-p10-release-migration/`: 10 small metadata/build-log evidence files (candidate bundles remain in preserved worktree).
- `evidence/agentdock-p11-adhoc-native-uat/`: 14 small metadata/build-log evidence files (candidate bundles remain in preserved worktree).
- `cleanup-selected.json`, `worktree-removal-results.tsv`: complete trace of removal reasons and outcomes.

The archive is local-only; this is **not** proof of off-machine/offsite backup. No branch was removed or pushed.

## Applied cleanup

31 clean worktrees were removed using `git worktree remove --force` **only after** checking their working-tree status, resolved path and one of: (1) ancestor of `custom/main`, or (2) `git cherry` showed no patch-unique commits. The second criterion was **not** treated as proof of a merge: historical branch refs and the backup bundle remain. All **31/31** operations returned success, and the common worktree list now has **14** members. No `git branch -D`, `git worktree prune` or `git gc` was run.

The largest reclaimed old tree, `agentdock-m8-permission-approval`, previously occupied **~15 GB**, essentially all under Git-ignored `dist/` temporary/repeated local build folders. Other removed merged/equivalent branches include historic M7, Wave 1, P5 and Shared Desktop worktrees, as well as already integrated NEXT Orca and 7-day retention rollout directories. Disk reclamation is not quoted as a precise total because no pre-operation filesystem-free baseline was measured.

## Preserved 14 worktrees and next disposition

| Worktree | Reason to keep | Next action |
| --- | --- | --- |
| `agentdock` | stable/upstream Git main | Maintenance-only, upstream updates only |
| `agentdock-custom` | active NEXT `custom/main` | Primary integration and current backlog |
| `agentdock-original-maintenance-20261021` | original dedicated maintenance line | Keep isolated from NEXT |
| `agentdock-m6-acp-tests` | 8 tracked + 9 untracked edits | Compare Browser/ACP changes with current P8 before cleanup |
| `agentdock-p5-mcp-oauth-fence` | 5 tracked edits | Reconcile OAuth fix against current P5/UX store |
| `agentdock-next-mcp-card-ux` | 1 unique P13/P14 prototype commit | Complete Host UAT, review and integrate |
| `agentdock-next-skills-cbm` | 2 unique CBM / Core Skills commits | Review package/live impact and integrate selectively |
| `agentdock-activity-center` | clean, patch-unique M5 history | Examine before archival, don't blindly merge old UI |
| `agentdock-next-orca-discovery-20261017` | clean, 3 patch-unique commits, overlaps later deployed Orca + CBM | Compare and close without regressions |
| `agentdock-p5-mcp-desktop-api` | clean, patch-unique history | Cross-check original P5 fixes vs newer integrated authority |
| `agentdock-p5-mcp-ui` | clean, patch-unique history | Cross-check original P5 fixes vs newer integrated UI |
| `agentdock-upstream-101-adoption` | clean, 4 patch-unique docs commits | Review relevant upstream adoption notes |
| `agentdock-p10-release-migration` | locked `initializing` in Git; local RC artifacts | Preserve; inspect lock owner/reason before removal |
| `agentdock-p11-adhoc-native-uat` | local Next-only install/rollback/GUI candidate evidence | Preserve while P11 native acceptance is unfinished |

## Next steps, in order

1. Continue from [current active NEXT backlog](next-active-backlog-2026-10-21.md) rather than stale historical P6/P8 checkpoint summaries.
2. Independently reconcile uncommitted M6 and P5 changes, then the two actual pending NEXT features (P13/P14 and Core Skills/CBM) in non-overlapping write scopes.
3. Triage the five patch-unique historical branches; once their unique changes are explicitly classified as integrated/obsolete/not needed, remove their now-clean worktrees while retaining or archiving refs.
4. Revisit P10 lock and RC bundle retention only after handoff of package/source provenance; do not break the native rollback evidence or current installed NEXT.
5. Protect the local-only NEXT history with a deliberate remote backup/push decision. **No push was authorized or executed in this cleanup.**

## Revalidation

Check `git worktree list --porcelain` contains 14 entries, clean `custom/main`, the two dirty worktrees unchanged, bundle verifies, and `curl -f http://127.0.0.1:{8765,8767}/healthz` succeeds independently. Never infer complete agent workflow acceptance from HTTP health alone.
