# aitv — 短剧内容上架、付费观看与下架处理（题目 04）

Gin + SQLite 实现的单部短剧内容运营闭环：内容编辑提交审核 → 审核通过 → 运营上架 →
用户试看/付费购买/观看 → 停止销售/暂停观看 → 退款与客服处理。完整需求见 `docs/requirements.md`。

## 技术栈

- Go 1.26 + Gin（Web 框架）
- SQLite（modernc.org/sqlite 纯 Go 驱动，无 CGO），单文件持久化，重启后业务结果可查
- 服务端渲染 HTML 最小业务页面（9 个页面 + 错误页）

## 运行

```bash
go run ./cmd/server -addr :8080 -db data/aitv.db
# 浏览器打开 http://localhost:8080
```

预置账号（密码均为 `pass123`）：

| 账号 | 角色 | 说明 |
| --- | --- | --- |
| editor | 内容编辑 | 提交审核、修订 |
| reviewer | 审核人员 | 通过/退回 |
| ops | 运营 | 上架/停售/暂停/恢复 |
| u1 / u2 / u3 | 用户 | 余额 500 / 300 / 100 平台币（u3 恰好够买一集，用于并发演示） |
| support | 客服 | 退款处理、权益核查 |

预置数据：1 部 6 集短剧，第 1-2 集免费，第 3-6 集各 100 平台币。

## 演示脚本

```powershell
# Windows：完整业务闭环演示（发布→购买→停售→暂停→退款→幂等）
powershell -ExecutionPolicy Bypass -File scripts/demo_flow.ps1 -BaseUrl http://localhost:8090
```

## 测试与质量门禁

```bash
go test ./... -coverprofile=coverage.out -covermode=atomic   # 覆盖率（整体与核心包 ≥85%）
go vet ./...
gofmt -l .
go test ./... -race                                           # 竞态检测（需 CGO，CI Linux 环境执行）
python skills/short-drama-consistency-check/scripts/check_consistency.py --db data/aitv.db  # 业务一致性核查
python skills/short-drama-acceptance-check/scripts/run_acceptance.py --base-url http://localhost:8080  # 验收检查
```

## 部署

- Docker：`docker build -t aitv . && docker run -p 8080:8080 -v aitv-data:/app/data aitv`
- 本地教学环境：`bash deploy/deploy_local.sh 8080 data`（构建→停旧→起新→健康检查→冒烟→失败回退）
- CI：`.github/workflows/ci.yml`（质量门禁→构建→部署→健康检查→业务冒烟，仅 main 触发部署）

## 目录结构

```
cmd/server/         入口
internal/store/     SQLite 连接、建表、预置数据
internal/biz/       业务错误与角色鉴权助手
internal/content/   剧集资料、提交审核、审核工作流、修订
internal/sales/     上架、停售、暂停、恢复、运营汇总
internal/billing/   购买扣币、权益、观看判定
internal/refund/    退款申请与客服处理、权益核查
internal/web/       Gin 路由、会话、页面与 JSON API
web/templates/      最小业务页面
skills/             两个自编 Skill（业务专项 + 验收检查）
docs/               需求、原型、任务、假设、测试与验收证据
deploy/             部署与冒烟脚本
```

## 过程材料索引

- 需求分析：`docs/requirements.md`；假设清单：`docs/assumptions.md`
- 原型设计：`docs/prototype.md`、`docs/prototype.html`
- 任务清单：`docs/tasks.md`
- Agent 约束：`AGENTS.md`
- Skill 使用：`docs/skill-usage.md`；AI 审核记录：`docs/ai-review-log.md`
- 测试证据：`docs/testing/`；部署说明：`deploy/`
