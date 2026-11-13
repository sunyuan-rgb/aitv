# 部署记录（教学验收环境）

## 1. 本次部署

| 项 | 内容 |
| --- | --- |
| 部署时间 | 2026-11-13 |
| 部署提交 | 最终发布版本 main `de8ca24`（CI run 43360117375 部署成功）；本地部署 dev/04 对应提交 |
| 制品 | `deploy/aitv-server.exe`（go build 生成，36.9 MB） |
| 部署方式 | `deploy/deploy_local.ps1` 等价步骤：构建 → 停旧进程 → 起新进程 → 健康检查 → 业务冒烟 |
| 数据目录 | `data/aitv.db`（SQLite 单文件，重启后业务数据保留） |
| 监听地址 | `:8080` |
| 健康检查 | `GET /login` → 200（证据见 `docs/deployment/smoke_after_deploy.txt`） |
| 业务冒烟 | 6/6 PASS：登录页、内容提交→审核→上架、免费集观看、购买幂等（余额不变）、已购观看、未购拒绝 |
| 冒烟脚本 | `deploy/smoke.ps1`（Windows）/ `deploy/smoke.sh`（Linux CI） |
| CI 部署 | run 43360117375（main `de8ca24`）success：质量门禁→构建→启动→健康检查→冒烟；首次失败记录 run 43354729950 与修复后 success 对照见 `docs/testing/ci_runs.txt` |
| 服务日志 | `deploy/app.log`（未入库，运行产物） |

> 说明：本地部署使用 dev/04 最终提交；GitHub Actions 在 main 上独立完成"构建 → 启动 → 健康检查 → 业务冒烟"，
> 两条路径的冒烟脚本内容一致（Windows PowerShell 版 / Linux Bash 版）。

## 2. 部署流程与失败回退

```powershell
powershell -ExecutionPolicy Bypass -File deploy/deploy_local.ps1 -Port 8080 -DataDir data
```

流程：构建失败直接退出；健康检查 30 次×1s 未通过则终止新进程（回退）；冒烟失败同样终止新进程并保留数据卷，可回滚到上一制品重新部署。

## 3. CI 流水线（GitHub Actions）

`.github/workflows/ci.yml`：

1. `quality-gate`：gofmt 检查、`go vet`、`go test -race -coverprofile`、覆盖率门禁（整体 + content/billing/sales/refund 各 ≥85%）、上传覆盖率产物。
2. `build`：`go build ./cmd/server`，上传制品。
3. `deploy`（仅 main 分支 push）：启动服务 → 健康检查 → `deploy/smoke.sh` 业务冒烟 → 写部署版本记录。

## 4. Docker 部署（可选）

```bash
docker build -t aitv .
docker run -d -p 8080:8080 -v aitv-data:/app/data --name aitv aitv
```

镜像内置 `HEALTHCHECK`（curl /login），数据目录挂载卷保证重启后业务结果可查。
