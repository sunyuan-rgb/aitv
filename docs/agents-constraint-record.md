# AGENTS.md 版本、加载与生效记录（题目 04）

## 1. 版本历史

| 版本 | 时间 | 变更 | 提交 | 说明 |
| --- | --- | --- | --- | --- |
| v1.0 | 2026-11-13 09:15（早于任何业务代码） | 首次编写并生效 | `d1a02d4` | 全文见仓库根 `AGENTS.md`；此后无调整 |

- 调整记录：**无**。若后续调整，须以 `docs` 类型提交并在此登记原因（规则见 `AGENTS.md` 第 6 节）。
- 校验命令：`git log --oneline -- AGENTS.md`（仅 1 条 docs 提交）。

## 2. 实际加载/读取记录

| 阶段 | 任务编号 | 会话/工具 | 读取到的关键约束 | 落地证据 |
| --- | --- | --- | --- | --- |
| 开发（编码前） | T4-T9 | CodeBuddy 开发会话（同一会话，读取 AGENTS.md 后开工） | 目录结构、Gin/SQLite 选型、事务与幂等、角色鉴权 | `internal/billing/billing.go` 事务与条件扣币；`internal/refund/refund.go` 只退一次 |
| 测试 | T11、T14 | 首次运行 Skill/测试前的自检 | 覆盖率门禁、禁止弱化断言 | `docs/testing/coverage_summary.txt`（整体 86.6%，核心包 ≥85%） |
| 验收 | T15 | 验收 Skill 会话（`skills/short-drama-acceptance-check`） | 逐项证据、不得依据完成声明判定 | `docs/skill_call_acceptance.txt`、`docs/acceptance/report.md` |
| 部署/合并 | T16-T18 | CI 与 PR 流程 | 禁止直接推送 main、PR 门禁、部署后健康检查 | `docs/testing/branch_protection.json`、`docs/testing/ci_runs.txt` |

## 3. 受约束的具体行为（举例）

1. **不得擅自扩大范围**：未实现转码、推荐、会员、充值；边界写入 `docs/requirements.md` 第 6 节。
2. **业务数据保护**：扣币与权益同事务（`billing.Buy`），退款退币与撤权益同事务（`refund.Decide`）。
3. **未明确事项记录假设**：A1-A11 见 `docs/assumptions.md`，未将猜测当作既定需求。
4. **禁止编造**：meegle 不可用时登记模拟编号与缺失证据（A7），GitHub MCP 建仓缺失登记于 A11。
5. **禁止为覆盖率删测试**：补充测试而非排除代码；覆盖率原始文件 `docs/testing/coverage.out` 入库。

## 4. 与任务清单的对应

`docs/tasks.md` 中 T1（约束编写）为本文件的前置任务；T4-T18 的完成条件均引用 `AGENTS.md` 的对应条款（质量门禁、提交规范、角色边界）。
