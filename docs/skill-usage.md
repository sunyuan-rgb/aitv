# skill-usage.md — 实际使用的 Skill 汇总（题目 04）

## 0. 工具中的配置方式

| 项 | 内容 |
| --- | --- |
| 工具 | CodeBuddy IDE Agent |
| 项目级 Skill 目录 | `.codebuddy/skills/<skill-name>/SKILL.md`（本仓库已放置两个自编 Skill 的可加载副本） |
| 源文件目录 | `skills/<skill-name>/`（评审阅读用，与 `.codebuddy/skills/` 内容一致） |
| 加载方式 | 在会话中通过 Skill 工具按 name 加载（`short-drama-consistency-check` / `short-drama-acceptance-check`），或在终端直接执行其 `scripts/` 脚本；本作业两种方式均实际使用 |
| 版本 | 两个 Skill 均为 v1.0；修改脚本时的版本变化记录在各自 SKILL.md 与 `docs/ai-review-log.md` |

## 1. short-drama-consistency-check（业务专项 Skill，自编）

| 说明项 | 内容 |
| --- | --- |
| 名称、来源与版本 | short-drama-consistency-check v1.0，自编（skill-creator init_skill.py 初始化骨架后定制）。文件：`skills/short-drama-consistency-check/`（SKILL.md、scripts/check_consistency.py、scripts/checks.sql） |
| 使用位置 | 开发完成后、部署冒烟阶段，对演示数据 `data/aitv.db` 执行；调用记录：`docs/skill_call_consistency.txt` |
| 使用原因 | 需要验证"内容生命周期、销售状态、观看权限、扣币与退款"在数据层面的跨环节一致性（题面规则1-8），人工逐表核对易遗漏，用确定性 SQL 脚本可重复执行并留证据 |
| 使用过程 | 输入：`--db data/aitv.db`（只读模式）。实际执行 10 条核查：审核留痕、修订单份限制、购买-权益一一对应、无重复扣币、余额非负、暂停期无新购、退款恰好撤一条权益、退款只退一次、价格不变、免费集无购买 |
| 实际帮助 | 10 项全部 PASS，确认演示数据满足全部业务约束；核对中发现并暴露了演示脚本"先停售/暂停后购买"的业务顺序错误（不是业务代码缺陷），指导修正脚本顺序后复检通过 |
| 证据及局限 | `docs/skill_call_consistency.txt`（10/10 PASS）、`docs/testing/demo_flow_output.txt`（修正后的完整流程）。局限：脚本只查数据层不变量，无法覆盖"并发时序正确性"，该部分依赖单元测试的并发用例与 CI 竞态检测 |

## 2. short-drama-acceptance-check（验收检查 Skill，自编）

| 说明项 | 内容 |
| --- | --- |
| 名称、来源与版本 | short-drama-acceptance-check v1.0，自编（skill-creator init_skill.py 初始化骨架后定制）。文件：`skills/short-drama-acceptance-check/`（SKILL.md、scripts/run_acceptance.py、references/acceptance_checklist.md） |
| 使用位置 | PR 合并前的 Agent 验收阶段，对 dev/04 构建与运行中服务（:8090）执行；调用记录：`docs/skill_call_acceptance.txt`、`docs/skill_call_acceptance_before_fix.txt` |
| 使用原因 | 题面要求"使用独立验收 Agent 依据原始题面逐项验证最终版本"，需要可重复的质量门禁（gofmt/vet/test/覆盖率）+ 部署冒烟 + 逐项证据清单 |
| 使用过程 | 输入：仓库根目录 + `--base-url http://localhost:8090`。执行 gofmt、go vet、go test、覆盖率门禁（核心包与整体 ≥85%）、登录/目录/我的记录冒烟；人工按 references/acceptance_checklist.md 核对 R1-R8 规则与五组场景 |
| 实际帮助 | **第一轮检查发现真实问题**：gofmt 报告 19 个文件格式不合规（见 before_fix 记录）；执行 `gofmt -w` 修复后复检，又暴露验收脚本自身两个缺陷（覆盖率误取函数级最大值、冒烟登录 Cookie/Content-Type 处理错误），修正 Skill 脚本后第三轮复检全部 PASS（整体覆盖率 86.6%，冒烟 5/5 PASS）。后续 PR #2 又修复了 CI 冒烟在全新环境缺少上架数据的问题（见 docs/ai-review-log.md #8） |
| 证据及局限 | 修复前：`docs/skill_call_acceptance_before_fix.txt`；修复后：`docs/skill_call_acceptance.txt`（结论"验收通过"，覆盖率门禁 PASS）。局限：脚本无法核对"讲解视频/群提交"等线下项，需人工按清单 C 项补充；race 检测依赖 CI（本机无 CGO 工具链，CI run 43356862931 已通过） |

## 3. skill-creator（既有 Skill，CodeBuddy 内置）

| 说明项 | 内容 |
| --- | --- |
| 名称、来源与版本 | skill-creator，CodeBuddy 内置 Skill |
| 使用位置 | 创建上述两个自编 Skill 时的初始化阶段 |
| 使用原因 | 按 `init_skill.py` 生成合规的 SKILL.md 骨架（frontmatter/目录结构），避免手工建目录漏项 |
| 使用过程 | 执行 `init_skill.py short-drama-consistency-check --path skills` 与 `init_skill.py short-drama-acceptance-check --path skills`，随后删除示例文件、填入用途/触发/输入/步骤/输出/失败处理/完成标准 |
| 实际帮助 | 两个 Skill 结构符合规范（name/description frontmatter 齐全，scripts/references 分类清晰），可被 CodeBuddy 以 Skill 方式加载 |
| 证据及局限 | `skills/` 目录结构。局限：仅提供骨架，业务内容与脚本均为本题自行编写 |

## Skill 检查发现问题 → 修正 → 复检 实例

1. **发现**（第 1 轮验收）：gofmt FAIL，19 个文件不合规 → 证据 `docs/skill_call_acceptance_before_fix.txt`。
2. **修正**：执行 `gofmt -w .`，重新构建确认无编译影响。
3. **复检**（第 2、3 轮）：复检中进一步暴露验收脚本覆盖率解析与冒烟 Cookie 缺陷，修正 Skill 脚本后 3/3 质量门禁 PASS + 4/4 冒烟 PASS → 证据 `docs/skill_call_acceptance.txt`。
