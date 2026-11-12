# NEXT RC11 — GitLab 專用 OAuth / MCP 唯讀 Host UAT（2026-10-24）

狀態：**NEXT OAuth + GitLab MCP 唯讀 Host PASS；GitLab REST token/read_api、寫入 API 不在這次驗收範圍**。

## 前提與隔離

- 使用者在 GitLab 完成 **NEXT-only** OAuth 同意後，以已安裝 NEXT `0.9.2-rc.11 / c331afe47d2e` 的 Core `127.0.0.1:8767` 上，使用其受保護的本機 MCP session 操作。`gitlab-company` 為獨立的 `streamable_http` connector；使用 NEXT local OAuth callback `127.0.0.1:8767/oauth/mcp/callback`，申請 `mcp` scope。**沒有擷取或複製原版 8765 的 grant、token、password；無 OAuth authorization URL、state、PKCE secret 留入 Git。**
- `mcp_manage list` 顯示 connector `ready`／**35 tools**；`mcp_manage refresh(name=gitlab-company)` 重新取得 **35 tools**、仍 `ready`。前一次的 `MCP_AUTH_REQUIRED` 在同一 NEXT connector 上已解除。
- GitLab MCP version：`19.5.1`，revision `449ed2875c2`。這是 server-returned version，不是 NEXT RC 版本。
- 所有實際操作都經 NEXT 自己的 `mcp_tool_inspect` 後 `mcp_tool_call(name=gitlab-company:<tool>)`，**不是** Stable 工具或本機 Git credential，也不是冒充獨立 GitLab REST personal access token 驗收。

## 唯讀 API 真實結果

| Gate | NEXT GitLab MCP 原生回應 | 結果 |
| --- | --- | --- |
| 專案發現 | `list_projects(search=oprueba)` 找到 `official_site/oprueba`，Project ID `251` | PASS |
| Project metadata | `get_project(project_id=251)` 返回路徑、default_branch、visibility、URL | PASS |
| Commits | `list_commits(first=3)` + `get_commit(6433255dd4df4f4e966fd6a2f013eaa069aa5967)` | PASS |
| Pipeline list | `list_pipelines(id=251, per_page=4)`；其中 staging `#3338` 是 `success` | PASS |
| Pipeline detail/jobs | `get_pipeline(id=251,pipeline_id=3338,include=[jobs])`；返回 8 個 jobs、分頁仍有下一頁 | PASS |
| Job detail | `get_job(id=251,job_id=8499)` 返回 `cms:reconcile`，`success` | PASS |
| Merge requests | `list_merge_requests(first=4)` + `get_merge_request(project_id=251, merge_request_iid=178)` | PASS |
| Branch list | `list_branches(id=251,search=staging)` 取得 staging 分支資訊 | PASS |

被動證據：GitLab web URLs 對應 `https://gitlab.oprueba.com/official_site/oprueba`、`/-/pipelines/3338`、`/-/jobs/8499`、`/-/merge_requests/178`。MR !178 讀取時為 `opened`（target `cms-content`）；這些是取樣當下狀態，未修改遠端專案。

## 保留之剩餘 gates

- **GitLab MCP 唯讀工作流程已 Host PASS**，不能寫成「待 OAuth」或「API 不能用」。如果未來產品需求要求獨立 GitLab REST API `read_api`、OAuth renew/revoke、token rotation、跨重啟恢復與角色權限等，仍需另案驗收；此次 `mcp` scope 不等同 REST `read_api`。
- NEXT 其他 RC gates：Edge 全部租約 release 後的 fresh acquire 原生 peer revalidation FAIL CLOSED、未核准 origin / 禁止 JS 的獨立負向、可見 GUI／Tray、故障／跨 session lifecycle、**RC11 live rollback**。四站／sibling／網站 no-focus 已另有 [Host 證據](next-rc11-host-uat-2026-10-23.md)。
- 只讀檢查；沒有 merge、push、deploy、approve、create branch、改權限或清除授權；原版 8765 維持原狀。
