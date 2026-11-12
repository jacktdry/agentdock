# NEXT RC12 候選前置：Edge peer revalidation 可靠性修正（2026-10-24）

狀態更新（2026-10-24）：**RC12 source、封裝與 NEXT-only 安裝 PASS，但修正版的 Edge Host lease admission FAIL CLOSED。** 早先 source-only 設計與測試記錄仍保留；已安裝實機證據詳見 [RC12 Host checkpoint](next-rc12-host-uat-2026-10-24.md)。Stable 8765／Edge Profile 沒有修改，也沒有重複偵錯連線。

## 產品問題與驗收契約

- 使用者工作流程：ChatGPT 與 ACP 可在**同一條經允許的 Edge Browser Broker 連線**依序取得／釋放獨立租約；上一組所有租約正常釋放後，新工作應可取得 fresh lease，而不是反覆要求使用者授權。
- 真正 Host 觀察：已安裝 RC11 完成 Oprueba／HomiPlay 與兩個 `www` origin 的導覽、AX、PNG，以及 A release→B snapshot→B release、no-focus **PASS**；之後新的 negative-only `direct_acquire` fail closed：`external peer verification unavailable or mismatched`。原 socket 在失敗後依然 TCP ESTABLISHED；只憑此不能確認 peer attestation 合法。當次依程序 disconnect，沒有重試。GitLab NEXT-only OAuth／MCP 唯讀已另行 Host PASS。
- 直接改善目標：減少**OS 證據收集與驗證排程**中的可避免誤拒；不變更 owner、origin、lease、process/profile/signer/held-socket 身分規則。
- 不做：不擴大公司 origin／Profile、不能降級到 Managed Chrome、不略過 macOS code signature、不延長 native noncooperative 作業或自動重連、不把 synthetic 測試當 Edge Host、不改裝原版。

## 最小 source 修改

1. `external_established_socket.go` 的 `edgeSocketProbe.connections` 改成接受已驗證的 `server.Port()`；`external_established_socket_darwin.go` 的 `lsof` 加 `-iTCP:<pinned_port>` 搭配既有 `-a -p PID -sTCP:ESTABLISHED -FpnT`。以前會讀 Edge **所有 ESTABLISHED TCP**，正常瀏覽產生大量 HTTPS 連線時可超過 8192 bytes／64 FD 限制，導致真實已授權的 CDP 連線被無關連線淹沒；現在僅讀偵錯埠，仍要求 exact reverse TCP tuple、same PID、雙次 lsof、雙次 process start token／live signature。
2. `external_process_preflight_darwin_cgo.go` 的 `edgeLiveSignatureVerifier.check` 在 worker 完成後，**先釋放唯一 global native slot，才發布成功結果**。舊版先發成功後釋放，下一個序列化 verify 可能在前一個已成功返回但 slot 尚佔用時被 `default` fail-closed；修正不改動真正並行或永久卡住時的 fail-closed。
3. 測試涵蓋：合成 socket/port attestation 改為必須使用 pinned port；macOS 使用**測試程序自己建立的兩個 loopback TCP 連線**驗證真實 lsof 僅回傳指定埠且 exact tuple 符合，從未存取 Edge；native slot 連續 150 次成功後沒有殘留占用；既有 panic／timeout／canceled verifier 保持拒絕。

## 已完成回歸（source，並非新 installed RC Host）

- `GOMAXPROCS=2 GOFLAGS=-p=2 go test ./internal/tool/browser ./internal/browserpolicy ./internal/config ./internal/app -count=1 -json`：**1,687 PASS、7 opt-in SKIP、0 FAIL**。
- 相關 `go test -race ./internal/tool/browser -run '^(TestEdgeEstablishedSocket|TestEdgeLiveSignature|TestEdgeEvidencePinned|TestEdgeEvidenceDirect|TestNativeEdgeFreshLeaseAfterAllOwnersRelease)' -count=1`：**85 PASS／0 FAIL**。
- macOS 真實 test-owned lsof scope：**PASS**；四包 `go vet`、`git diff --check`：**PASS**。
- 私有測試紀錄保存在 `/private/tmp/agentdock-next-edge-verification-y6b6836m/rc12-four-package-tests.jsonl` 與 `rc12-peer-targeted-race.jsonl`。不可當成與使用者目前 Edge 協作工作已修復的證據。

## 下一個最近產品 gate

清潔提交後建立 **NEXT-only RC12 local candidate**，以同一個 canonical source commit 打包／驗證 ad-hoc signature、Core/App version、ZIP/DMG；先不安裝／不 push。後續在確定 NEXT 無活躍 Browser／Computer／ACP／OAuth 工作與 user consent 的窗口，才能 NEXT-only 部署；一次正式原生 Edge connect 後優先測 **A/B release→new lease→兩項分離 negative check**，舊 RC11 的四站成功紀錄保留而不浪費反覆導覽。不讓沒有驗證的候選阻擋獨立 GUI／Tray、異常恢復與 rollback gates。
