# 验收核对清单（依据题目04原始题面逐项核对）

> 验收 Agent 使用：每条必须给出证据位置（代码路径/测试名/命令输出/接口响应），不得依据完成声明判定通过。

## A. 业务规则（题面规则 1-8）

| 编号 | 核对项 | 证据位置（填写） |
| --- | --- | --- |
| R1 | 编辑维护资料并提交审核；仅审核通过可首次上架；退回后可修改重提 | internal/content/content.go（SubmitForReview/reviewEpisode）；测试 TestSubmitAndReviewFlow |
| R2 | 已上架内容修改须重新审核（未公开修订）；每集最多一份待审修订 | internal/content/content.go（revision 分支）；测试 TestRevisionFlow |
| R3 | 用户仅能浏览已上架内容；免费集直接观看；付费集购买产生单集权益 | internal/billing/billing.go（Watch/GetPublished）；测试 TestWatchPermissionMatrix |
| R4 | 价格上架后固定；重复购买不重复扣币；并发购买不超扣；扣币与权益原子 | internal/billing/billing.go（Buy 事务）；测试 TestBuyIdempotent、TestConcurrentBuyOneAffordable |
| R5 | 停止销售仅禁新购（老用户可看）；暂停观看禁购买+播放；须填写原因并展示 | internal/sales/sales.go；测试 TestStopSalesAndPause、TestPauseBlocksAll |
| R6 | 暂停期间已购用户可申请退款；客服确认只退一次并撤销权益；免费集/未购不可申请 | internal/refund/refund.go；测试 TestSupportApproveRefundOnce、TestRequestRefundValidation |
| R7 | 恢复后未退款用户恢复观看；已退款用户旧权益不可用但可重新购买；历史记录保留 | internal/billing/billing.go；测试 TestSupportApproveRefundOnce（重购分支） |
| R8 | 运营可查每集状态/购买人数/待处理退款；客服可解释权益；用户只看自己记录 | internal/sales（OpsSummary）、internal/refund（Explain）；测试 TestOpsSummaryAndLogs、TestPendingListAndExplain |

## B. 五组完整工作场景

| 场景 | 核对要点 | 证据位置（填写） |
| --- | --- | --- |
| 发布与消费 | 编辑提交→退回→重提→通过→上架→试看→付费→播放 | web 测试 TestFullUserFlowViaAPI |
| 停售分支 | 停售→新购被拒→老用户可看→恢复后新购成功 | sales 测试 TestStopSalesAndPause |
| 暂停分支 | 暂停→申请退款→客服处理→恢复后已退/未退权限不同 | refund 测试 TestSupportApproveRefundOnce |
| 并发及权限 | 购买与停售/暂停并发、重复购买、余额只够一集并发买两集、重复退款、自审、越权 | billing 测试 TestConcurrentBuyOneAffordable、TestConcurrentSameRequestId；refund 测试 TestConcurrentDecideOnlyOnce；content 测试 TestSelfReviewForbidden |
| 运营收尾 | 每用户扣币/退款/权益可核查；历史不因状态变化丢失 | billing MyRecords、sales OpsSummary；测试 TestMyRecordsWithRefunds |

## C. 共同要求核对

| 编号 | 核对项 | 结论 |
| --- | --- | --- |
| C1 | 最小业务页面可发起业务/查询/展示失败原因 | 9 个模板页 + 错误页 |
| C2 | 数据库持久化，重启后结果可查 | SQLite 文件 + store 测试 TestOpenFileDBAndSeed |
| C3 | 单元测试整体与核心包 ≥85% | run_acceptance.py 覆盖率门禁 |
| C4 | gofmt / go vet / race（CI）通过 | run_acceptance.py + .github/workflows/ci.yml |
| C5 | AGENTS.md 存在且生效 | 仓库根 AGENTS.md |
| C6 | 两个自编 Skill + 使用记录 | skills/ 目录 + docs/skill-usage.md |
| C7 | CI/CD：质量门禁/覆盖率/构建/部署/健康检查/冒烟 | .github/workflows/ci.yml、deploy/ |
| C8 | 提交规范与分支流程 | git log 与 PR 记录 |
