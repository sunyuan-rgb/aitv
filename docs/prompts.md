# 全过程提示词与可见会话导出（题目 04）

> 工具：CodeBuddy（IDE Agent）。模型版本：**不可获取**（IDE 未暴露）。
> 会话时间：2026-11-13。本文件按下述规则导出；平台隐藏指令与模型隐藏推理不在导出范围。

## 0. 导出说明与缺失范围

- 已导出：学生输入的提示词原文、按阶段的 AI 可见操作摘要（工具调用与命令输出、失败与重试）、
  纠错要求、关联提交版本。
- 完整逐字会话（含 AI 全部自然语言回复）由 IDE 会话面板保存，**无法以文件形式导出**，此处以阶段摘要补齐；
  原始命令输出已随证据文件入库（`docs/testing/`、`docs/skill_call_*.txt`、`docs/testing/ci_runs.txt`）。
- 结构化事件清单见 `docs/events.jsonl`；AI/学生分工见 `docs/ai-usage.md`。

## 1. 学生提示词（原文）

**P1（初始任务，唯一一条主提示词）**

```text
你是一个GO语言开发工程师,根据Go项目场景文档中的 题目 04：短剧内容上架、付费观看与下架处理,
文档中有题目应用的10条要求,1从实现范围与环境,2必须完成的开发流程,3. AI 规则约束,4. 自行编写并使用 Skill,
5. 导出全过程提示词及 AI 使用记录,6. 测试与 AI 产出质量要求,7. 自动部署与最终提交,
8. GitHub 仓库、开发分支与合并流程,9. Git 提交规范与工作项校验,严格完成每个要求,使用gin框架开发,
其中项目的技术栈,功能,业务都要有.代码的文件夹中要有agent的规则约束,在编写skill上 @skill://skill-creator
创建两个,1业务专项 Skill,2验收检查 Skill,文档要求中有详细要求,两个skill在什么时候使用效果好你直接自己用就行.
克隆https://github.com/sunyuan-rgb/aitv.git在此仓库中开发,完善细节避免有漏掉的各项需求.
```

**P2（附件上下文）**：`Go项目业务场景题.md`（含题目 04 与共同要求 1-10）、`commit-convention.md`（提交规范）。

> 说明：本次会话学生未追加新的提示词；后续纠错通过学生审阅结果后的口头/文档要求体现（见第 3 节与
> `docs/ai-review-log.md`）。故不存在"补充上下文提示词文本"，不虚构。

## 2. 阶段摘要（AI 可见操作）

| 阶段 | 时间 | 学生输入 | AI 主要可见操作与输出 | 关联版本 |
| --- | --- | --- | --- | --- |
| 环境与建仓 | 09:06-09:20 | P1 | 读取题面与规范；`git clone` 直连失败 3 次 → 改用 `gh-proxy.com` 镜像成功；创建 `dev/04`；确认 Go 版本与模块代理 | Initial commit `5929b51` |
| 约束与需求 | 09:20-10:00 | P1 | 编写 `AGENTS.md`（角色边界/Git 规范/数据保护/质量门禁/AI 红线）；产出 `requirements.md`、`assumptions.md`（A1-A10）、`prototype.md/html`、`tasks.md` | `d1a02d4` |
| 业务开发 | 10:00-10:30 | P1 | 建 SQLite schema 与预置数据；实现 content/sales/billing/refund 四个业务域与 Gin 路由、9 个页面；多轮 `go build` 修复编译错误 | `6dd6cf5`…`e769de1` |
| 测试与修复 | 10:30-10:36 | P1 | 编写 7 个包测试；首次 `go test` 出现 12 处失败（非法状态、余额、模板路径、JSON 字段大小写），逐项修复；补充边界/错误分支测试；覆盖率 86.6%、核心包均 ≥85% | `13b0831` |
| Skill 建设与使用 | 10:36-10:40 | P1 + `skill-creator` | 用 `init_skill.py` 生成两个 Skill 骨架并定制脚本；在演示数据上执行一致性核查（10/10 PASS）；执行验收 Skill，首轮发现 gofmt 不合规 → 修复 → 复检 | `920476c`、`docs/skill_call_*.txt` |
| CI/部署 | 10:37-10:50 | P1 | 编写 `ci.yml`（质量门禁/覆盖率/构建/部署/健康检查/冒烟）、Dockerfile、deploy 脚本；本地部署健康检查 200；PR #1 合并 | `b1d3e80`、`bb9082d` |
| 部署失败与修复 | 10:48-11:05 | 学生审阅 CI 结果 | main CI 部署作业失败（全新库无上架数据）→ 新增内容/审核 API、冒烟脚本自建数据并比较余额 → PR #2 合并 → main CI 全绿 | `4823ec7`、`602423a` |
| 验收与收尾 | 11:05-11:40 | 学生要求补齐文档 | 修复目录接口泄露草稿字段（DTO）；补充运行说明、验收报告、部署记录、CI 证据；本地重部署 + 冒烟 6/6；整理提示词与事件清单 | `036955e`、`237db9d` 及后续 docs 提交 |

## 3. 纠错要求与 AI 响应（有记录依据）

| # | 学生要求/触发 | AI 响应 | 证据 |
| --- | --- | --- | --- |
| 1 | 直连 GitHub 克隆失败 | 改用镜像完成克隆并在文档记录网络限制 | `docs/git-flow.md` 说明 |
| 2 | 验收 Skill 报告 gofmt 不合规 | 立即 `gofmt -w` 并复检 | `docs/skill_call_acceptance_before_fix.txt` |
| 3 | main CI 部署冒烟失败 | 增加内容/审核 API、冒烟自建数据、修复后 PR #2 合并 | `docs/testing/ci_runs.txt` |
| 4 | 联调发现目录接口返回草稿字段 | 改为用户视图 DTO 并补测试 | `TestEpisodesAPIHidesDraftFields` |
| 5 | 学生要求不得伪造工作项核验 | 在 `docs/assumptions.md` A7 登记模拟编号与 meegle 不可用事实 | `AGENTS.md` 第 2 节 |

## 4. 附件与索引

- 题面：`Go项目业务场景题.md`（外部附件，未入库）
- 提交规范：`commit-convention.md`（外部附件，未入库；要点已转述于 `docs/git-flow.md`）
- 原型：`docs/prototype.html`
- 测试与覆盖率原始输出：`docs/testing/test_output.txt`、`coverage_raw.txt`、`coverage.out`
- Skill 调用记录：`docs/skill_call_consistency.txt`、`docs/skill_call_acceptance.txt`
- CI 运行：`docs/testing/ci_runs.txt`、`.github/workflows/ci.yml`
