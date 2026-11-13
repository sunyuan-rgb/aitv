#!/usr/bin/env python3
"""通过 GitHub API 创建 PR 并合并（凭据取自 git credential fill，不输出凭据）。

用法: python scripts/create_pr.py [--merge]
"""
import json
import subprocess
import sys
import urllib.error
import urllib.request

REPO = "sunyuan-rgb/aitv"
HEAD = "dev/04"
BASE = "main"
TITLE = "[feat] 题目04 短剧内容上架、付费观看与下架处理"
BODY = (
    "题目04 完整实现与验收材料：\n\n"
    "- Gin + SQLite，内容编辑/审核/上架、试看/付费购买/观看、停售/暂停/恢复、退款/客服\n"
    "- 两个自编 Skill：业务一致性检查、验收检查（含调用记录）\n"
    "- 质量门禁：gofmt / go vet / go test（整体与核心包覆盖率 ≥85%）\n"
    "- CI 流水线、Dockerfile、部署与业务冒烟脚本\n"
    "- 验收报告：docs/acceptance/report.md\n"
)


def token() -> str:
    p = subprocess.run(
        ["git", "credential", "fill"],
        input="protocol=https\nhost=github.com\n\n",
        capture_output=True,
        text=True,
    )
    for line in p.stdout.splitlines():
        if line.startswith("password="):
            return line[len("password="):]
    sys.exit("未能获取 GitHub 凭据（git credential fill 无结果）")


def api(method: str, path: str, tk: str, payload=None):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(
        f"https://api.github.com{path}",
        data=data,
        method=method,
        headers={
            "Authorization": f"token {tk}",
            "User-Agent": "aitv-dev",
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.getcode(), json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read().decode() or "{}")


def main() -> int:
    tk = token()
    code, pr = api("POST", f"/repos/{REPO}/pulls", tk,
                   {"title": TITLE, "head": HEAD, "base": BASE, "body": BODY})
    if code not in (200, 201):
        # 已存在则查询打开中的 PR
        code2, prs = api("GET", f"/repos/{REPO}/pulls?head={REPO.split('/')[0]}:{HEAD}&state=open", tk)
        if code2 == 200 and prs:
            pr = prs[0]
        else:
            print(f"创建 PR 失败: {code} {pr.get('message')}")
            return 1
    print(f"PR #{pr['number']} {pr['html_url']} state={pr['state']}")

    if "--merge" in sys.argv:
        code, res = api("PUT", f"/repos/{REPO}/pulls/{pr['number']}/merge", tk,
                        {"merge_method": "merge",
                         "commit_title": f"Merge pull request #{pr['number']} from {HEAD}"})
        if code == 200 and res.get("merged"):
            print(f"已合并: {res['sha']} {res['message']}")
            return 0
        print(f"合并失败: {code} {res.get('message')}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
