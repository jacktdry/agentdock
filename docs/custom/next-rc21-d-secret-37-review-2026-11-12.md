# AgentDock NEXT — D 37 credential heuristic candidates: bounded manual semantic review

> 2026-11-12. **Only the 37 generic assignment hits in newly reachable blobs (relative to `origin/custom/main`) are in scope.** Original blob hash prefix and line number are identifiers, not current HEAD line numbers. No secret values or code lines are reproduced here. This is an offline source review, not an external secret-scanner certification.

## Reviewed method and result

- Revisited **all 37 prior heuristic matches** with original immutable Git blob content, inspected their immediate statement context, identified the right-hand-side expression semantics, and grouped duplicate versions of the same source logic. All 37 entries are accounted for below.
- The **25 production OAuth code matches** reference runtime variables / struct members, the six Desktop matches are method pointers, two Swift Installer matches reference variables, and two PowerShell matches are function calls reading or generating a secret at runtime. These are **not hardcoded production credentials**.
- The two `_test.go` matches are test-only: one dynamic URL using a test token payload and one fixed synthetic fixture token. The latter is a literal but is not a production credential; this is a bounded semantic determination based on surrounding test context.
- **Result for the 37 candidates: 37 reviewed / 37 explained / 0 evidence of a real committed production token within these matches.** Keep earlier high-confidence-pattern scan and its limitations; do not assert no secret exists in any other blob or format. No code correction or rotation needed on these 37 findings.

## Individual review ledger

Code categories: **O1** OAuth registration/token variable/field (24), **O2** OAuth persisted store variable/field (1), **S1** macOS Installer variable (2), **S2** Desktop ReadPassword interface (6), **P1** PowerShell function call (2), **T1** test URL data (1), **T2** test fixture literal (1). **37 total.**

| # | Historical path | Git blob prefix:line | Class | Verified meaning |
|---|---|---|---|---|
| 01 | `desktop/macos/AgentDockApp/Sources/InstallerRunner.swift` | `ba6cc57016b4:129` | S1 | No actual secret observed — value references a runtime OAuth password variable, not a quoted credential |
| 02 | `desktop/macos/AgentDockApp/Sources/InstallerRunner.swift` | `f489db9c3f64:123` | S1 | No actual secret observed — value references a runtime OAuth password variable, not a quoted credential |
| 03 | `internal/desktopapi/connection_auth.go` | `833d262abeec:42` | S2 | No actual secret observed — assignment references a ReadPassword function/method, not a secret literal |
| 04 | `internal/desktopapi/connection_auth.go` | `833d262abeec:54` | S2 | No actual secret observed — assignment references a ReadPassword function/method, not a secret literal |
| 05 | `internal/desktopapi/connection_auth.go` | `3b1ea8fe482e:40` | S2 | No actual secret observed — assignment references a ReadPassword function/method, not a secret literal |
| 06 | `internal/desktopapi/connection_auth.go` | `3b1ea8fe482e:52` | S2 | No actual secret observed — assignment references a ReadPassword function/method, not a secret literal |
| 07 | `internal/desktopapi/connection_auth.go` | `8a57dd8da373:30` | S2 | No actual secret observed — assignment references a ReadPassword function/method, not a secret literal |
| 08 | `internal/desktopapi/connection_auth.go` | `8a57dd8da373:42` | S2 | No actual secret observed — assignment references a ReadPassword function/method, not a secret literal |
| 09 | `internal/httpx/oauth_flow_test.go` | `20766bd89d70:127` | T1 | No actual secret observed — test builds query from an in-memory token payload (httptest); no credential literal |
| 10 | `internal/mcp/client/manager_test.go` | `7db1a4ebb061:1489` | T2 | No actual secret observed — fixed test fixture string in a _test.go file; synthetic, not runtime credential |
| 11 | `internal/mcp/oauthclient/manager.go` | `2afa44a16d27:575` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 12 | `internal/mcp/oauthclient/manager.go` | `2afa44a16d27:634` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 13 | `internal/mcp/oauthclient/manager.go` | `2afa44a16d27:771` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 14 | `internal/mcp/oauthclient/manager.go` | `2afa44a16d27:892` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 15 | `internal/mcp/oauthclient/manager.go` | `9dc63c4b8d9d:572` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 16 | `internal/mcp/oauthclient/manager.go` | `9dc63c4b8d9d:631` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 17 | `internal/mcp/oauthclient/manager.go` | `9dc63c4b8d9d:768` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 18 | `internal/mcp/oauthclient/manager.go` | `9dc63c4b8d9d:889` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 19 | `internal/mcp/oauthclient/manager.go` | `603711a4ed55:572` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 20 | `internal/mcp/oauthclient/manager.go` | `603711a4ed55:631` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 21 | `internal/mcp/oauthclient/manager.go` | `603711a4ed55:768` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 22 | `internal/mcp/oauthclient/manager.go` | `603711a4ed55:889` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 23 | `internal/mcp/oauthclient/manager.go` | `bb5c57a931f0:526` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 24 | `internal/mcp/oauthclient/manager.go` | `bb5c57a931f0:585` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 25 | `internal/mcp/oauthclient/manager.go` | `bb5c57a931f0:722` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 26 | `internal/mcp/oauthclient/manager.go` | `bb5c57a931f0:843` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 27 | `internal/mcp/oauthclient/manager.go` | `1c95e34e01cd:427` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 28 | `internal/mcp/oauthclient/manager.go` | `1c95e34e01cd:486` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 29 | `internal/mcp/oauthclient/manager.go` | `1c95e34e01cd:623` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 30 | `internal/mcp/oauthclient/manager.go` | `1c95e34e01cd:744` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 31 | `internal/mcp/oauthclient/manager.go` | `7fad4ca8fb85:376` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 32 | `internal/mcp/oauthclient/manager.go` | `7fad4ca8fb85:431` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 33 | `internal/mcp/oauthclient/manager.go` | `7fad4ca8fb85:568` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 34 | `internal/mcp/oauthclient/manager.go` | `7fad4ca8fb85:688` | O1 | No actual secret observed — OAuth registration/token fields are sourced from Go variables or struct members |
| 35 | `internal/mcp/oauthclient/store.go` | `6e0289b48996:328` | O2 | No actual secret observed — Go struct AccessToken is initialized by an existing variable |
| 36 | `scripts/install/install.ps1` | `cd886610b647:354` | P1 | No actual secret observed — PowerShell references a password read/generate function, not a committed value |
| 37 | `scripts/install/install.ps1` | `cd886610b647:363` | P1 | No actual secret observed — PowerShell references a password read/generate function, not a committed value |

## Publication and repository governance decision

- This resolves the **37-item candidate-review blocker** left open in [D governance report](next-rc21-d-github-governance-closeout-2026-11-12.md); no positive findings arose from those matches. It does **not** give blanket clearance for arbitrary future pushes or Git history rewrites.
- GitHub repository is **public and maintained by one collaborator/owner**. No second reviewer is available or required. Do not mandate `required_pull_request_reviews >= 1` or unknown required GitHub checks (Actions shows no runs). Proposed eventual protected `custom/main`: disable force pushes and branch deletion, **approval count 0**, allow the owner to use optional PR and check actual CI before merge; any remote settings change is separate from this offline integration and should be verified after application.
- This review and the D governance documents are now in the isolated locally merged RC21 source baseline; **no remote GitHub settings were modified and no Push/Tag/Release was performed**.
