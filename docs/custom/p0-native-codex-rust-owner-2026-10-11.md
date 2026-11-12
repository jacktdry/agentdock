# P0 HIGH-0 原生 Codex MCP Owner：0.160.0 定位與安全實作契約

狀態：**真實 Native Rust 來源已定位；Thread→transport→PID/pipe 原生對應尚未實作或驗證。不可宣告 orphan/cleanup 完成。**

## 可重現的 Native baseline

- 目前 `codex --version`：`codex-cli 0.160.0`。對應 upstream tag：`rust-v0.160.0`，本機隔離 sparse clone：`/Users/wei/sideProject/codex-native-mcp-owner`（基底 commit `a956835d`）。**該目錄不是目前使用中的 `codex` 安裝位置**，不得取代 `/opt/homebrew/bin/codex`。
- [MCP stdio launcher](https://github.com/openai/codex/blob/rust-v0.160.0/codex-rs/rmcp-client/src/stdio_server_launcher.rs)：`LocalStdioServerLauncher::launch_server` 目前從 `LocalStdioTransport::spawn` 得到 `transport.id()`，還在本地保有 process handle / terminate ownership；其 `StdioServerProcessHandle` 未對上層暴露不可偽造的 `child birth + stdio pipe identity`。
- [rmcp client](https://github.com/openai/codex/blob/rust-v0.160.0/codex-rs/rmcp-client/src/rmcp_client.rs)：`RmcpClient.stdio_process` 持有該 handle，但外部目前只能操作 MCP workflow，無 `thread + server + transport PID/pipe` 診斷 snapshot。
- [Thread context](https://github.com/openai/codex/blob/rust-v0.160.0/codex-rs/core/src/codex_thread.rs)：`mcp_server_status_snapshot` 可讀 per-thread runtime 對應的 MCP 狀態；需繼續向 `codex-rs/codex-mcp` 中的 runtime server registry 加入精確 transport-generation identity，而不是把 server name 當成 ownership proof。
- [Native app-server](https://github.com/openai/codex/blob/rust-v0.160.0/codex-rs/app-server/src/request_processors/mcp_processor.rs)：`mcpServerStatus/list` 目前偏向工具、連線狀態與配置，沒有完整 child PID/birth/pipe 的可信回傳。當前 JS ACP fork `bba784e` 的 HMAC extension 只能讀到這個 native RPC，**`mcpChildPIDAndTransport` 一律 `unavailable`，`reclaimPermission=false`**。

## 原生整合必需改動

1. 在唯一 native MCP launcher **立即建立**本地 child 時記錄一個不可猜測的 runtime transport generation，以及 OS process birth（PID、start time、UID/parent、platform-specific process handle）。Unix 的 native stdin/stdout pipe identity 必須能由 Kernel 的 FD observer 再次驗證；不能只靠 `transport.id()` 或 PID。Executor/remote MCP、HTTP、Plugin 或其他 APP 不得偽裝 local child。
2. 傳遞 read-only 身分從 launcher → `RmcpClient` → per-thread `codex_mcp` runtime registry → app-server private read-only diagnostic。結果要包含完整 thread/session generation、thread-to-transport mapping、child birth、pipe object identity，且 atomic snapshot 不因 startup/reload/close/child respawn 遺漏任何子連線。status-only/非完整資料必須 fail closed。
3. JS ACP fork 只負責 Next-owned pinning、nonce/HMAC、限額與重播拒絕；Go `internal/acp/mcp_owner_native_verified.go` 現在的 partial evidence **禁止被升級成 verified child mapping**，除非 `internal/mcpowneraudit/correlation.go` 的 native+Darwin kernel pipe proof、全部 birth/owner/epoch/exclusivity 條件真正通過。
4. 僅在 trusted Core+M8/P5 lifecycle policy 取得單一直接 process handle 所有權之後才能考慮 cleanup；任何外部、來源不明或共享進程仍顯示 `owner_unverified`。`session/close`／`thread/unsubscribe` 本身不保證 native thread/stdio child 已卸載。

## 驗收與阻塞

需對獨立測試 binary 完成：20 輪 create / call / unsubscribe / close / app-server restart；真實 stdio pipe FD/Kernel birth 驗證；PID reuse、同名 server、shared / remote / HTTP / plugin exclusion、runtime reload race 與 Windows Job Object 例外。不能因 native app-server 可回 PID 就授予自動清理權。

本機 Rust toolchain `cargo`／`rustc` 尚未在 Mac-Dev PATH 被觀測到，因此**目前沒有 Native Rust 編譯通過或 native patch 完成的證據**。先維持 isolated source baseline、Go 的 read-only/status-only 檢查以及既有生產 binary 不變；切勿將 `cargo test` 標記為已通過。
