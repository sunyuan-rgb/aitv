---
name: short-drama-acceptance-check
description: 验收检查 Skill：依据题目04原始题面逐项验收短剧平台实现，核对需求实现、异常路径、测试证据与部署结果。在 Agent 验收阶段或 PR 合并前使用，输出带证据位置的逐项验收报告。
---

# 短剧平台验收检查（题目 04）

## 用途

对照原始题面（题目04：短剧内容上架、付费观看与下架处理）与共同要求，逐项验证：
需求实现完整性、异常路径、单元测试覆盖率门禁、静态检查、部署与冒烟结果。
验收 Agent 必须读取原始题面与当前版本代码，**不得依据开发方声明直接判定通过**。

## 触发场景

- 功能完成、准备发起 PR 合并前
- 合并到 main 后对发布版本的复验
- 教师验收前的自检

## 输入

- 仓库根目录（默认当前目录）
- `--base-url`：（可选）运行中服务的地址，用于部署冒烟；缺省跳过部署项并标注 SKIP
- 题面文件：`docs/requirements.md`（含题面 8 条规则与 5 组场景映射）

## 执行步骤

1. 运行自动化验收脚本（质量门禁 + 覆盖率门禁 + 部署冒烟）：

   ```bash
   python skills/short-drama-acceptance-check/scripts/run_acceptance.py [--base-url http://localhost:8080]
   ```

2. 脚本依次执行：
   - `gofmt -l`（应无输出）、`go vet ./...`
   - `go test ./...`（含并发用例）
   - `go tool cover -func` 解析各包覆盖率，核心包（content/billing/sales/refund）与整体须 ≥85%
   - `--base-url` 提供时：登录、目录、购买幂等、停售/暂停约束、退款流程冒烟
3. 依据 `references/acceptance_checklist.md` 逐条人工核对业务场景（每条给出证据位置）。
4. 输出验收报告到 `docs/acceptance/report-<日期>.md`。

## 输出格式

```text
== 质量门禁 ==
gofmt ............. PASS
go vet ............ PASS
go test ........... PASS (xx tests)
覆盖率: content 86.2% billing 89.1% sales 86.3% refund 87.6% 整体 86.2% ... PASS
== 部署冒烟 ==
健康检查 .......... PASS
== 逐项核对 ==
[R1] 审核通过才可上架 .............. 通过 证据: internal/content/reviewEpisode + 测试 TestSubmitAndReviewFlow
...
结论: 验收通过 / 验收不通过（缺失项列表）
```

## 失败处理

- 任一质量门禁失败：验收不通过，报告中列出命令与原始输出，交回开发修复；修复后必须重新运行受影响检查。
- 覆盖率低于 85%：验收不通过；禁止通过删除测试或排除生产代码来达标。
- 冒烟失败：记录接口与响应，判定部署项不通过。
- 无法执行的项标注 SKIP 并说明原因，不得标记为通过。

## 完成标准

质量门禁全 PASS、核心包与整体覆盖率 ≥85%、逐项核对全部给出证据位置且无未解释的 SKIP；
报告留档并可追溯到具体提交哈希。
