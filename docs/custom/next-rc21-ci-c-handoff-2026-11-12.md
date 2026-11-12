# AgentDock NEXT RC21 — C Windows Permission Durability HANDOFF (2026-11-12)

## Scope and result

- **Owner:** C only, Windows Core Permission Store; no Codex ACP delegation.
- **User/runtime pain:** Hosted Windows full Go Test ([Run 43143453728](https://github.com/jacktdry/agentdock/actions/runs/43143453728)) repeated `fsync permissions directory ... Access is denied` (~197 occurrences), blocking the same Permission Store lifecycle across tests.
- **Expected improvement:** bootstrap/reload/policy and approval commits on Windows no longer call `os.File.Sync` on a directory; all state transitions retain pre-publication file flush, private state ACLs, same-directory replacement and fail-closed handling of uncertain commit results.
- **Base:** `b7a26cd9f5fded276f6459622bb6be4916c5caca` (fixed, not rebased).
- **Branch:** `fix/next-rc21-ci-c-win-permission-20261112`
- **Worktree:** `/Users/wei/sideProject/agentdock-next-rc21-ci-c-win-permission-20261112`
- **Implementation commit:** `51b38a5c3387706082904d529a495b4013afe85f`
- **C HANDOFF:** this file, separately committed after the implementation so that the above implementation SHA is exact; see `git log -2` for this document commit.

This addresses the **M8 Permission Store / RC21 Hosted Windows Go Test gate**, not native Windows application release, installer, crash/power-loss certification or full Browser/ACP acceptance.

## Files changed (strict C ownership)

- `internal/permission/store.go`: secure existing/new directory and state with shared `internal/fs/securepath.EnsurePrivate`, secure temp file *before* file `Sync` and replace, platform dispatch, read-only disk state privacy checks, quarantine Store on uncertain replacement results. `Policy()` returns an invalid policy during quarantine so policy evaluation cannot silently authorize; Snapshot, Approval, History, VerifyDiskState and further persistence reject with `ErrStateCorrupt`. Recovery requires constructing a fresh Store from validated on-disk data.
- `internal/permission/store_replace_windows.go`: same-directory `windows.MoveFileEx` with `MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH`; no `MOVEFILE_COPY_ALLOWED`, no copy/delete fallback, no Windows directory `Sync` and **no error swallowing**. Errors are surfaced and quarantine the old Store.
- `internal/permission/store_replace_unix.go`: existing same-directory rename + real directory `Sync`; now opening the directory also fails explicitly instead of being ignored.
- `internal/permission/store_private_windows.go`: read-only protected DACL verification, allowed full-access ACE principals limited to current user, SYSTEM and Administrators, with expected directory inheritance; no reliance on meaningless Windows `os.FileMode().Perm()==0600`.
- `internal/permission/store_private_unix.go`: verify private `0700` directory and `0600` state file.
- `internal/permission/store_test.go`: private state assertion becomes platform-correct.
- `internal/permission/store_durability_test.go`: simulated failure **before** replacement confirms no disk/memory policy commit, cleanup and quarantine; simulated reported error **after** real replacement confirms stale memory cannot authorize, fresh Store reload recovers.
- `internal/permission/store_private_unix_test.go`: reject widened state file/directory permissions.
- `internal/permission/store_private_windows_test.go`: Windows native private ACL before/after replace, intentionally unsafe ACL rejected, repaired ACL trusted.
- `internal/permission/store_replace_windows_test.go`: native Windows cross-directory move rejected with source intact.

No other source, fixtures, CI YAML, shared plan, Original, Stable or active NEXT runtime files were modified. The shared `securepath` implementation is **used unchanged**.

## Verification and evidence (Go 1.26.5 only)

| Platform | Command | Result | Evidence level |
|---|---|---|---|
| macOS ARM64 | `GOTOOLCHAIN=go1.26.5 go test -race ./internal/permission ./internal/fs/securepath -count=1 -timeout=5m` | **PASS** | Native Mac, includes new positive/negative tests |
| macOS ARM64 | `GOTOOLCHAIN=go1.26.5 go vet ./internal/permission ./internal/fs/securepath` | **PASS** | Native Mac static check |
| Windows amd64 | `GOOS=windows GOARCH=amd64 GOTOOLCHAIN=go1.26.5 go test -c -o /tmp/agentdock-rc21-c-permission.test.exe ./internal/permission` | **PASS** | Windows PE test binary compiled on Mac; **NOT executed on Windows** |
| Windows amd64 | `GOOS=windows GOARCH=amd64 GOTOOLCHAIN=go1.26.5 go vet ./internal/permission` | **PASS** | Cross-platform static analysis only |
| Linux amd64 | `GOOS=linux GOARCH=amd64 GOTOOLCHAIN=go1.26.5 go test -c -o /tmp/agentdock-rc21-c-permission.test-linux ./internal/permission` | **PASS** | Compiled, **NOT executed on Linux** |
| Hosted Windows / Linux / macOS CI | No CI jobs triggered by C | **NOT TESTED** | Must be executed by integration owner later |

`git diff --check` and implementation commit scope check passed; implementation commit contains exactly 10 `internal/permission/**` files. This HANDOFF is intentionally outside that implementation commit, at its own C-only permitted path.

## Windows Hosted Runner MUST VERIFY before releasing C gate

1. On **GitHub `windows-latest`**, record `go version` as **go1.26.5**, then run `go test ./internal/permission -count=1 -timeout=10m`; require all tests pass, not just a successful cross-build. Re-run targeted state/approval tests (`go test ./internal/permission -run 'Test(Store|Permission|VerifyDisk|PriorEpoch|Approve|Consume)' -count=10 -timeout=10m`) to catch file-lock or rename timing issues.
2. Confirm creation, read, replacement, policy changes and reload work without the old `fsync permissions directory ... Access is denied` message. No silent bypass of `file.Sync`, ACL or replace errors; sharing violations must remain actionable failures rather than fallback success.
3. Run the native Windows `TestPermissionWindowsPrivateDACLAndTamperFailClosed` and `TestPermissionWindowsReplaceRefusesCrossDirectoryMove`: state and directory ACL must be protected and private; tampering must cause `ErrStateCorrupt`, and a cross-directory rename must not be converted to copying. Also run `go test ./internal/fs/securepath -count=1` for reused protected-ACL helper.
4. Native Windows positive/negative durability tests must establish pre-replacement failure leaves prior JSON intact and no temp files; ambiguous *post*-replacement failure quarantines the live Store, rejects stale policy and prevents grants until a new Store is loaded. Validate restart/epoch recovery never resurrects one-shot approvals.
5. Run the integrated full Windows Go Test job after bringing in **both C and D**. Separate C Permission Store failures from D Browser Profile/Scope, release and fixture errors; do **not** treat fewer downstream failures as proof D passed.
6. Review filesystem context: supported local NTFS permissions, expected SID/ACL inheritance, file sharing/Defender behavior. A genuine process crash and unexpected power-loss durability verification remains separate native/VM work; Hosted unit tests cannot prove it.
7. Require subsequent PR #1 Hosted **Linux, macOS, Windows, Browser and Container** gates before integration. Only the integration owner may adjust CI workflow, push/update the PR or merge.

## Durability contract, limits and handoff risks

- **Write sequence:** encode/validate → same-directory `O_EXCL` temp → set private ACL/mode → `file.Sync()` (must succeed) → `Close()` (must succeed) → atomic replacement → publish memory only after success. Failure before replacement leaves previous authoritative state intact; failure from replacement is treated as *indeterminate*, quarantines in-memory authorization, requires validated reload. No attempt to 'undo' an uncertain disk operation by an unsafe second overwrite.
- Windows uses `MoveFileExW` with `MOVEFILE_WRITE_THROUGH`, following the existing local precedent in `internal/selfupdate/apply_windows.go`. Microsoft documents the flags at https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw ; importantly the documented flush guarantee explicitly discusses **copy-and-delete** moves. It does **not** by itself prove every filesystem/device's same-volume rename survives an unexpected power failure. We therefore **do not claim power-loss certification**; native Windows tests and any stronger product durability commitment remain an explicit gate.
- `FlushFileBuffers` is a file flush API, not a portable Windows directory-fsync equivalent: https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers . Do not replace this with unconditional ignores of permission or sync failures.
- Windows NTFS ACL enforcement requires ownership/permissions to set protected DACL. Unsupported ACL filesystems or denied security operations should fail closed; do not silently downgrade to chmod-only privacy. There is no Windows host accessible in this C session, so Windows ACL behavior and `MoveFileEx` test assertions are **compile-verified only**.
- Unix directory `Sync` after rename can report failure after the new file became visible; the quarantine makes the live Store fail closed. We did **not** add a general cross-process file lock or redesign the multi-Core authority model.

## Integration boundary and next action

The integration session should cherry-pick the implementation commit **and** this HANDOFF commit into its own independent Integration Worktree; retain A/B/D ownership. Execute Hosted Windows full tests using **true Windows Runner**; only then resolve the Windows Permission Gate, correlate the old ~197 repeated failures, and check D's remaining Windows errors. No push, merge, CI edit, deployment or branch/worktree cleanup was carried out here. Existing stable Mac-Dev, Original and active NEXT services remained untouched.
