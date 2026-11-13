# AI 使用与人工审核记录（题目 04）

学生按 AGENTS.md 要求审核 AI 产出。以下为实际发现的 AI 产出错误、修正要求与复测记录。

| # | 阶段 | AI 产出问题 | 修正要求 | 复测结果 |
| --- | --- | --- | --- | --- |
| 1 | 开发 | 初版 `Watch`（internal/billing）在暂停观看分支直接返回，未向已退款用户说明"权益已随退款撤销"，退款场景信息不完整 | 暂停分支增加已退款状态识别与提示 | 测试 `TestWatchPausedRefundedUser` 通过；演示输出可看到退款提示 |
| 2 | 开发 | 初版预置数据 u3 余额 50，无法满足"余额恰好够买一集时并发买两集"场景 | 按《假设清单》A2 修正为 100 | `TestConcurrentBuyOneAffordable` 通过（并发恰好成功 1 次，余额 0） |
| 3 | 开发 | 初版 `PublishRevision` 修订状态扫描与状态机校验混乱（int 扫描+兜底查询） | 重写为文本状态直读 + 状态机前置校验 | sales 测试全通过，覆盖修订发布全部分支 |
| 4 | 开发 | AI 编写的演示脚本顺序错误：先停售/暂停后购买，导致演示流程出现预期外的失败 | 指导调整为"先购买后停售/暂停"，并增加幂等重试演示 | `docs/testing/demo_flow_output.txt` 全流程符合预期 |
| 5 | 测试 | 测试包中混入未使用的辅助类型（idHelper 等残留代码） | 删除残留，统一 `bizCode` 助手 | go vet 通过 |
| 6 | 验收 | 验收 Skill 脚本 v1 自身缺陷：覆盖率取函数级最大值、冒烟未处理 Cookie/Content-Type | 修正 Skill 脚本（包级覆盖率解析 + CookieJar + 表单类型区分） | 第 3 轮复检全 PASS（docs/skill_call_acceptance.txt） |
| 7 | 验收 | gofmt 报告 19 个文件格式不合规 | `gofmt -w .` 后复检 | gofmt PASS |
| 8 | 部署 | main `bb9082d` 的 CI 部署作业失败：全新库中无已上架剧集，冒烟脚本假设演示数据存在 | 新增内容提交/审核 JSON API，冒烟脚本自建数据并兼容已有数据（比较前后余额） | main `602423a` 的 CI 全流程 success（run 43356862931） |
| 9 | 联调 | `/api/episodes` 直接序列化内部模型，返回了草稿字段（`CurTitle` 等），违反"用户仅能浏览已上架内容" | 改为用户视图 DTO（id/seq/title/is_free/price/play_url/status） | 新增 `TestEpisodesAPIHidesDraftFields` 通过；目录接口不再出现草稿字段 |

## 约束文件版本与生效说明

- `AGENTS.md` 在业务开发开始前编写并提交（见提交历史中最早的 docs 提交），开发、测试、验收三个阶段均以其为约束。
- 调整记录：无中途调整；假设事项全部记录在 `docs/assumptions.md`（A1-A10），未经确认的猜测未进入实现。
- 学生实际审核内容：上表 7 项 + 全部提交前 `git diff` 复查 + 测试报告与验收报告的原始输出核对。
