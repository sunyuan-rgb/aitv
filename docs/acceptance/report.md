# 验收报告 — 题目 04 短剧内容上架、付费观看与下架处理

- 被验收版本：main `de8ca24`（PR #4 合并后的发布版本）；本地验收对应 dev/04 同内容提交
- 自动化门禁复验：main CI run 43360117375 success（质量门禁 + 构建 + 部署 + 冒烟）
- 验收方式：独立验收 Skill（`skills/short-drama-acceptance-check`）+ 依据原始题面逐项人工核对
- 验收时间：2026-11-13
- 验收环境：Windows 本机教学验收环境，服务 `http://localhost:8080`，SQLite `data/aitv.db`
- 自动化输出：`docs/skill_call_acceptance.txt`；单元测试输出：`docs/testing/test_output.txt`；覆盖率明细：`docs/testing/coverage_summary.txt`；CI 运行记录：`docs/testing/ci_runs.txt`

## 1. 质量门禁

| 门禁 | 结果 | 证据 |
| --- | --- | --- |
| gofmt | PASS（无未格式化文件） | `docs/skill_call_acceptance.txt` |
| go vet | PASS（无告警） | 同上 |
| go test（7 个包） | PASS | 同上 |
| 覆盖率：整体 | 86.6%（≥85%） | `docs/testing/coverage_summary.txt` |
| 覆盖率：content | 86.2%（≥85%） | 同上 |
| 覆盖率：billing | 89.1%（≥85%） | 同上 |
| 覆盖率：sales | 86.3%（≥85%） | 同上 |
| 覆盖率：refund | 87.6%（≥85%） | 同上 |
| 覆盖率：web | 89.1% | 同上 |
| 竞态检测 `-race` | 由 CI Linux 执行（run 43356862931 全流程 success） | `.github/workflows/ci.yml`、`docs/testing/ci_runs.txt`；本机无 CGO 工具链，另以确定性并发用例兜底（TestConcurrentBuyOneAffordable / TestConcurrentSameRequestId / TestConcurrentDecideOnlyOnce） |

## 2. 业务规则逐项验收（对应题面 8 条）

| 编号 | 结论 | 证据位置 |
| --- | --- | --- |
| R1 编辑提交审核、仅审核通过可上架、退回可重提 | 通过 | `internal/content/content.go:SubmitForReview/reviewEpisode`；`content.TestSubmitAndReviewFlow`、`TestSelfReviewForbidden`；演示输出第 1 段 |
| R2 已上架修改须重新审核（未公开修订）、每集最多一份待审 | 通过 | `internal/content/content.go` 修订分支；`content.TestRevisionFlow`；`sales.TestPublishRevision`（运营发布后线上版本才替换） |
| R3 仅浏览已上架；免费集直看；付费集购买生成权益 | 通过 | `internal/billing/billing.go:Watch/GetPublished`；`billing.TestWatchPermissionMatrix`；`TestWatchDraftEpisode` |
| R4 价格固定、重复购买不重复扣币、并发不超扣、扣币与权益原子 | 通过 | `billing.Buy`（条件更新 + 事务）；`TestBuyIdempotent`、`TestConcurrentBuyOneAffordable`（余额 100 并发两集恰成功 1 次）、`TestConcurrentSameRequestId`（10 并发仅 1 条记录）、`TestRaceDetectorConcurrentBuys` |
| R5 停售仅禁新购、暂停禁购买+播放、须填原因并展示 | 通过 | `internal/sales/sales.go:StopSales/Pause`；`sales.TestStopSalesAndPause`、`TestPauseBlocksAll`（拒绝原因含运营原因）；`billing.TestWatchShowsReasons` |
| R6 暂停期退款、客服确认只退一次并撤权益、免费集/未购不可申请 | 通过 | `internal/refund/refund.go`；`refund.TestSupportApproveRefundOnce`、`TestRequestRefundValidation`、`TestRequestRefundFreeEpisode`、`TestConcurrentDecideOnlyOnce`（3 并发批准恰 1 次） |
| R7 恢复后未退款可看、已退款需重购、历史保留 | 通过 | `billing.Watch` 权益判定；`refund.TestSupportApproveRefundOnce`（恢复→重购→2 条购买记录→可看） |
| R8 运营查询状态/购买人数/待处理退款；客服解释权益；用户仅看自己 | 通过 | `sales.OpsSummary`、`refund.Explain`、`billing.MyRecords`；`sales.TestOpsSummaryWithPausedAndRefund`、`refund.TestPendingListAndExplain`、`billing.TestMyRecordsWithRefunds` |

## 3. 五组完整工作场景

| 场景 | 结论 | 证据 |
| --- | --- | --- |
| 发布与消费 | 通过 | `web.TestFullUserFlowViaAPI`（提交→通过→上架→购买→观看）；`docs/testing/demo_flow_output.txt` |
| 停售分支 | 通过 | `sales.TestStopSalesAndPause`；演示输出"u2 buy stopped ep5 (should FAIL)" + "u1 watch stopped ep5 allowed" |
| 暂停分支 | 通过 | `refund.TestSupportApproveRefundOnce`；演示输出退款前后余额与观看判定 |
| 并发及权限 | 通过 | 并发用例 4 个 + 自审 `TestSelfReviewForbidden` + 越权 `web.TestRoleGuardsOnPages`、`refund.TestPendingListAndExplain` |
| 运营收尾 | 通过 | `sales.OpsSummary` 汇总与 `RecentLogs` 留痕；`billing.MyRecords` |

## 4. 共同要求核对

| 项 | 结论 | 证据 |
| --- | --- | --- |
| 最小业务页面可发起/查询/展示失败原因 | 通过 | `web/templates/` 9 页 + error.html；`web.TestSupportRefundViaPages`（观看被拒页展示原因） |
| 数据持久化、重启可查 | 通过 | `store.TestOpenFileDBAndSeed`（关闭重开余额仍在）；`data/aitv.db` |
| 单元测试整体与核心包 ≥85% | 通过 | 见第 1 节 |
| 部署 + 健康检查 + 冒烟 | 通过 | 本地：`docs/deployment/record.md`、`docs/deployment/smoke_after_deploy.txt`（6/6 PASS）；CI：run 43360117375 success |
| 主分支保护 | 通过 | `docs/testing/branch_protection.json`（HTTP 200：禁止强推/删除、要求 PR 与质量门禁状态检查） |
| AGENTS.md 约束 | 通过 | 仓库根 `AGENTS.md` |
| 两个自编 Skill 及使用记录 | 通过 | `skills/`、`docs/skill-usage.md`、`docs/skill_call_consistency.txt`、`docs/skill_call_acceptance.txt` |
| CI 质量门禁/构建/部署 | 通过（配置） | `.github/workflows/ci.yml`；本机 CI 等价命令已在验收中执行 |

## 5. 未通过 / 需人工补充项

- `go test -race` 需在 CI（Linux）执行，本机 CGO 缺失：SKIP 已说明，不作为通过依据；CI 流水线包含该步骤。
- 讲解视频、群提交等线下项不在自动化范围，由学生按课程要求补充。

## 6. 验收结论

**验收通过**：8 条业务规则、5 组场景、质量门禁与部署冒烟均有可追溯证据。
