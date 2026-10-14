# P10 Windows source-only feasibility acceptance — 2026-10-14

> **使用者最新決定（取代較早文件中 Windows 原生驗收的必過要求）：** 現階段沒有可提供測試的 Windows 設備；Windows 平台只要求**理論可行性、來源與交叉編譯證據**，不做 Windows 實機、VM、安裝／移除、WPF GUI、Windows/WSL native UAT。除非使用者未來另行決定，這些測試不是 P10 closeout 或 P11 的必過條件。

## 接受範圍與未宣稱範圍

- **P10 主線發佈目標：macOS arm64。** 版本、Build Info、候選 Manifest、ZIP/DMG、更新來源防護與 CI 的 macOS 驗證依既有安全標準完成；UI/UX 與公開發佈時機仍由使用者另行決定，P11 簽章／更新／回滾不提前實施。
- **Windows 為 non-shipping feasibility track（不阻塞 macOS P10）。** 以 Go amd64/arm64 交叉編譯、PE/GOOS/GOARCH/SHA/UTC Build Info、WPF/Inno/PowerShell 原始碼審查、隔離與負向測試、離線候選雜湊清單作為理論可行證據。已確認過的來源驗證可以計入 Windows feasibility 完成度，但不等同在 Windows 編譯或實測 WPF／Setup。
- Windows Next 控制面板仍 **fail-closed**；`NextAgentDock.iss` 的無條件 `#error` 與 `prepare-next-rc.py --build-installer` 的拒絕必須保留。沒有正式 Next Windows 安裝包、Windows 可用性保證、啟動／更新／移除能力。既有 Stable Windows 程式／路徑／排程工作絕不可被 Next 探測或變更。
- **Windows 不發布、不開通更新 feed**；P10 release Manifest 的 `planned_artifacts` 僅列 macOS arm64 ZIP／DMG，Windows Setup 名稱只保留在 Windows 理論可行性工具／歷史文件作未來規劃，不是這次 P10 發佈產物，也不是 Release 權限。`windows_version` 保留為跨平台來源版本規格，不代表 Windows 可交付。若日後要提供 Windows 正式版，應另立明確平台交付／驗收工作線，再取得足以支持產品可用性的實證，不得僅靠此 source-only 決定宣稱已驗收。
- WSL native UAT 依賴 Windows host，同樣不作為本階段強制門檻。Linux-only helper 可以依獨立可用環境驗證，不冒充 Windows/WSL 實機。

## 重新計算 P10 closeout

1. **Windows 不再阻塞：** 不要求 Windows Runtime/Installer 完整生命周期、Windows Setup、Windows WPF/ISCC 原生編譯、GitHub Hosted Windows runner、Windows/WSL native UAT 或 PE Authenticode 作為 macOS-focused P10 closeout 的必要條件。這些原始碼未完成／未驗收狀態仍應如實記錄，不能把風險移到 Windows 使用者身上。
2. **仍要完成的 macOS P10 工作：** 與最終已提交 SHA 相符的 Next macOS arm64 App／ZIP／DMG 及版本/包名/雜湊檢查；Next-only source/Release Pipeline 的 local/Hosted macOS CI 證據（如未推送無法執行，應明列未跑）、必要的 source integration 與 closeout 文件。
3. **發佈等待條件不變：** UI/UX 尚未凍結，不建立正式 Git Tag／GitHub Release、不 Push／安裝／啟用 Next 正式更新 feed；P10 可完成 source/prepublish 工程交付而正式對外發佈仍暫緩。
4. **P11 界線不變：** macOS Developer ID/notarization、更新／回滾／恢復與授權的原生驗收照原計畫另行處理。Windows/WSL native UAT 改列「設備可用且使用者另行授權才進行」，而非強制 gate。

本決策優先於較早 P10/P11 checkpoint 文字內「Windows Installer/native/Hosted Windows CI 未完成就不能 P10 closeout」的要求；**不回溯竄改早期測試事實、也不將未實作功能標成完成**。
